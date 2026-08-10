package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", ":12802", "listen address")
	configPath := flag.String("config", "./configs/river.toml", "river.toml path")
	restartName := flag.String("restart-container", "go-mysqlbinlog-to-es", "docker container name to restart")
	staticDir := flag.String("static", "", "static web root (default: auto-detect)")
	syncLogURL := flag.String("sync-log-url", "http://127.0.0.1:12801/logs", "sync process in-memory /logs URL")
	flag.Parse()

	webRoot := *staticDir
	if webRoot == "" {
		for _, p := range []string{"static", "web/admin/static", "../../web/admin/static"} {
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				webRoot = p
				break
			}
		}
		if webRoot == "" {
			webRoot = "static"
		}
	}

	s := &Server{
		ConfigPath:       *configPath,
		RestartContainer: *restartName,
		SyncLogURL:       *syncLogURL,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/config/raw", s.handleConfigRaw)
	mux.HandleFunc("/api/config/backups", s.handleConfigBackups)
	mux.HandleFunc("/api/config/restore", s.handleConfigRestore)
	mux.HandleFunc("/api/sync-state", s.handleSyncState)
	mux.HandleFunc("/api/schemas", s.handleSchemas)
	mux.HandleFunc("/api/tables", s.handleTables)
	mux.HandleFunc("/api/columns", s.handleColumns)
	mux.HandleFunc("/api/json-keys", s.handleJSONKeys)
	mux.HandleFunc("/api/restart", s.handleRestart)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.Handle("/", http.FileServer(http.Dir(webRoot)))

	log.Printf("go-mysqlbinlog-to-es admin listening on %s, config=%s, static=%s", *addr, *configPath, webRoot)
	if err := http.ListenAndServe(*addr, withCORS(mux)); err != nil {
		log.Fatal(err)
	}
}

type Server struct {
	ConfigPath       string
	RestartContainer string
	SyncLogURL       string
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		c, err := loadConfig(s.ConfigPath)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	case http.MethodPut, http.MethodPost:
		var c Config
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		backupNote, err := saveConfig(s.ConfigPath, &c)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		msg := "已写入 river.toml。若同步容器正在运行，请点击「重启同步容器」使配置生效。"
		if backupNote != "" {
			msg = backupNote + " " + msg
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"ok":      "saved",
			"path":    s.ConfigPath,
			"backup":  backupNote,
			"message": msg,
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleConfigRaw(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(s.ConfigPath)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"path":    s.ConfigPath,
			"content": string(data),
		})
	case http.MethodPut, http.MethodPost:
		var body struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(body.Content) == "" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("content 不能为空"))
			return
		}
		backupNote, err := saveConfigRaw(s.ConfigPath, []byte(body.Content))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		msg := "已按原文写入 river.toml。若同步容器正在运行，请点击「重启同步容器」使配置生效。"
		if backupNote != "" {
			msg = backupNote + " " + msg
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"ok":      "saved",
			"path":    s.ConfigPath,
			"backup":  backupNote,
			"message": msg,
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSyncState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	c, err := loadConfig(s.ConfigPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	dataDir, masterPath, hasMaster := syncStateFromConfig(c)
	hint := "无位点（下次启动将全量 dump）"
	if hasMaster {
		hint = "已有位点（下次启动将跳过 dump，从 binlog 继续）"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data_dir":         dataDir,
		"master_info_path": masterPath,
		"has_master_info":  hasMaster,
		"hint":             hint,
	})
}

func (s *Server) handleConfigBackups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	list, err := listConfigBackups(s.ConfigPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"backups": list,
		"max":     maxConfigBackups,
		"hint":    "bak.1 为最近一次保存前的版本，bak.3 为最旧",
	})
}

func (s *Server) handleConfigRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Slot int `json:"slot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := restoreConfigBackupSafe(s.ConfigPath, body.Slot); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"ok":      "restored",
		"message": fmt.Sprintf("已回退到 river.toml.bak.%d。请点击「重启同步容器」使配置生效。", body.Slot),
	})
}

func (s *Server) handleSchemas(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	c, err := loadConfig(s.ConfigPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	db, err := openMySQL(c)
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("连接 MySQL 失败: %w", err))
		return
	}
	defer db.Close()

	list, err := listSchemas(db)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"schemas": list})
}

func (s *Server) handleTables(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	schema := r.URL.Query().Get("schema")
	if schema == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("缺少 schema"))
		return
	}
	c, err := loadConfig(s.ConfigPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	db, err := openMySQL(c)
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("连接 MySQL 失败: %w", err))
		return
	}
	defer db.Close()

	list, err := listTables(db, schema)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tables": list})
}

func (s *Server) handleColumns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	schema := r.URL.Query().Get("schema")
	table := r.URL.Query().Get("table")
	sampleJSON := r.URL.Query().Get("sample_json") != "0"
	if schema == "" || table == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("缺少 schema 或 table"))
		return
	}
	c, err := loadConfig(s.ConfigPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	db, err := openMySQL(c)
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("连接 MySQL 失败: %w", err))
		return
	}
	defer db.Close()

	cols, err := listColumns(db, schema, table)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if sampleJSON {
		for i := range cols {
			if !cols[i].IsJSON {
				continue
			}
			keys, err := sampleJSONKeys(db, schema, table, cols[i].Name, 20)
			if err != nil {
				log.Printf("sample json keys %s.%s.%s: %v", schema, table, cols[i].Name, err)
				continue
			}
			cols[i].JSONKeys = keys
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"columns": cols})
}

func (s *Server) handleJSONKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	schema := r.URL.Query().Get("schema")
	table := r.URL.Query().Get("table")
	column := r.URL.Query().Get("column")
	if schema == "" || table == "" || column == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("缺少 schema/table/column"))
		return
	}
	c, err := loadConfig(s.ConfigPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	db, err := openMySQL(c)
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("连接 MySQL 失败: %w", err))
		return
	}
	defer db.Close()

	keys, err := sampleJSONKeys(db, schema, table, column, 30)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"keys": keys})
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		FullDump bool `json:"full_dump"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	name := s.RestartContainer
	errs := []string{}

	// Full dump must stop the sync process BEFORE deleting master.info.
	// Otherwise the shutting-down process rewrites the position file on exit
	// and the next start still "skip dump".
	if body.FullDump {
		c, err := loadConfig(s.ConfigPath)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, fmt.Errorf("读取配置失败，无法全量 dump: %w", err))
			return
		}
		dataDir, _, _ := syncStateFromConfig(c)

		if name == "" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("全量 dump 需要配置 restart-container（Docker 先停再删位点）"))
			return
		}
		if err := dockerStop(name); err != nil {
			writeErr(w, http.StatusBadGateway, fmt.Errorf("停止同步容器失败: %w", err))
			return
		}
		p, err := clearMasterInfo(dataDir)
		if err != nil {
			_ = dockerStart(name) // best-effort bring back
			writeErr(w, http.StatusInternalServerError, fmt.Errorf("删除 master.info 失败: %w", err))
			return
		}
		log.Printf("full_dump: stopped %s, removed %s", name, p)
		if err := dockerStart(name); err != nil {
			writeErr(w, http.StatusBadGateway, fmt.Errorf("已删除位点 %s，但启动容器失败: %w", p, err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"ok":        "restarted",
			"method":    "docker-stop-clear-start",
			"container": name,
			"message":   fmt.Sprintf("已停止容器、删除 %s 并重新启动，将执行全量 dump。", p),
		})
		return
	}

	// Normal restart: reload config / reconnect without clearing position.
	if name != "" {
		if err := dockerRestart(name); err != nil {
			errs = append(errs, "docker: "+err.Error())
			log.Printf("docker restart failed: %v", err)
		} else {
			writeJSON(w, http.StatusOK, map[string]string{
				"ok":        "restarted",
				"method":    "docker",
				"container": name,
				"message":   "已通过 Docker API 重启同步容器，新配置即将生效。",
			})
			return
		}
	}

	if err := syncSelfRestart(s.SyncLogURL); err != nil {
		errs = append(errs, "self: "+err.Error())
		writeErr(w, http.StatusBadGateway, fmt.Errorf("重启失败: %s", strings.Join(errs, " | ")))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"ok":      "restarted",
		"method":  "self-exit",
		"message": "已通知同步进程退出，Docker 将自动重新拉起（若 docker API 不可用则走此方式）。",
	})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "buffer"
	}
	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "300"
	}
	filter := r.URL.Query().Get("filter")

	switch source {
	case "docker":
		lines, err := dockerLogs(s.RestartContainer, tail, filter)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"source": "docker",
			"lines":  lines,
		})
	case "buffer", "memory":
		base := strings.TrimRight(s.SyncLogURL, "/")
		if base == "" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("未配置 sync-log-url"))
			return
		}
		reqURL := fmt.Sprintf("%s?tail=%s&filter=%s", base, tail, url.QueryEscape(filter))
		resp, err := http.Get(reqURL)
		if err != nil {
			writeErr(w, http.StatusBadGateway, fmt.Errorf("读取进程内日志失败（请确认同步服务 /logs 可访问）: %w", err))
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			writeErr(w, http.StatusBadGateway, fmt.Errorf("sync /logs HTTP %s: %s", resp.Status, string(body)))
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	default:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("source 仅支持 buffer 或 docker"))
	}
}

func dockerLogs(container, tail, filter string) ([]string, error) {
	if container == "" {
		return nil, fmt.Errorf("未配置 restart-container")
	}
	client, err := dockerHTTPClient()
	if err != nil {
		return nil, err
	}
	ver := dockerAPIVersion(client)
	apiURL := fmt.Sprintf("http://localhost/%s/containers/%s/logs?stdout=1&stderr=1&timestamps=1&tail=%s", ver, container, tail)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Docker logs 失败（请确认已挂载 /var/run/docker.sock）: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("docker logs failed: %s %s", resp.Status, string(body))
	}

	raw := decodeDockerLogStream(body)
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	f := strings.ToLower(strings.TrimSpace(filter))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if f != "" && !strings.Contains(strings.ToLower(line), f) {
			continue
		}
		out = append(out, line)
	}
	return out, nil
}

// Docker multiplexes stdout/stderr with an 8-byte header per frame.
func decodeDockerLogStream(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	// Heuristic: if it looks multiplexed (stream byte 0-2), demux; else treat as plain text.
	if len(b) >= 8 && b[0] <= 2 && b[1] == 0 && b[2] == 0 && b[3] == 0 {
		var out strings.Builder
		i := 0
		for i+8 <= len(b) {
			size := int(b[i+4])<<24 | int(b[i+5])<<16 | int(b[i+6])<<8 | int(b[i+7])
			i += 8
			if size < 0 || i+size > len(b) {
				out.Write(b[i:])
				break
			}
			out.Write(b[i : i+size])
			i += size
		}
		return out.String()
	}
	return string(b)
}

func dockerHTTPClient() (*http.Client, error) {
	sock := os.Getenv("DOCKER_HOST")
	if sock == "" {
		sock = "unix:///var/run/docker.sock"
	}
	if !strings.HasPrefix(sock, "unix://") {
		return nil, fmt.Errorf("仅支持 unix socket，当前 DOCKER_HOST=%s", sock)
	}
	path := strings.TrimPrefix(sock, "unix://")
	transport := &http.Transport{
		Dial: func(_, _ string) (net.Conn, error) {
			return net.DialTimeout("unix", path, 3*time.Second)
		},
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}, nil
}

func dockerAPIVersion(client *http.Client) string {
	for _, u := range []string{
		"http://localhost/v1.44/version",
		"http://localhost/v1.41/version",
		"http://localhost/version",
	} {
		resp, err := client.Get(u)
		if err != nil {
			continue
		}
		var info struct {
			APIVersion string `json:"ApiVersion"`
		}
		err = json.NewDecoder(resp.Body).Decode(&info)
		resp.Body.Close()
		if err == nil && info.APIVersion != "" {
			return "v" + info.APIVersion
		}
	}
	return "v1.44"
}

func dockerRestart(container string) error {
	return dockerContainerAction(container, "restart", "t=5")
}

func dockerStop(container string) error {
	return dockerContainerAction(container, "stop", "t=8")
}

func dockerStart(container string) error {
	return dockerContainerAction(container, "start", "")
}

func dockerContainerAction(container, action, query string) error {
	client, err := dockerHTTPClient()
	if err != nil {
		return err
	}
	// stop/start/restart may take longer than default client timeout
	client.Timeout = 60 * time.Second
	ver := dockerAPIVersion(client)
	apiURL := fmt.Sprintf("http://localhost/%s/containers/%s/%s", ver, container, action)
	if query != "" {
		apiURL += "?" + query
	}
	req, err := http.NewRequest(http.MethodPost, apiURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("调用 Docker API 失败（请确认已挂载 /var/run/docker.sock）: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// 204 No Content / 304 already started are success
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotModified {
		return fmt.Errorf("docker %s failed: %s %s", action, resp.Status, string(body))
	}
	return nil
}

func syncSelfRestart(syncLogURL string) error {
	base := strings.TrimRight(syncLogURL, "/")
	base = strings.TrimSuffix(base, "/logs")
	if base == "" {
		return fmt.Errorf("未配置 sync-log-url，无法自重启")
	}
	apiURL := base + "/admin/restart"
	resp, err := http.Post(apiURL, "application/json", strings.NewReader("{}"))
	if err != nil {
		return fmt.Errorf("请求同步进程 %s 失败: %w", apiURL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("self-restart HTTP %s: %s", resp.Status, string(body))
	}
	return nil
}
