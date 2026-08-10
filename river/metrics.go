package river

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/siddontang/go-log/log"
)

var (
	esInsertNum = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mysql2es_inserted_num",
			Help: "The number of docs inserted to elasticsearch",
		}, []string{"index"},
	)
	esUpdateNum = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mysql2es_updated_num",
			Help: "The number of docs updated to elasticsearch",
		}, []string{"index"},
	)
	esDeleteNum = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mysql2es_deleted_num",
			Help: "The number of docs deleted from elasticsearch",
		}, []string{"index"},
	)
	canalSyncState = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "mysql2es_canal_state",
			Help: "The canal slave running state: 0=stopped, 1=ok",
		},
	)
	canalDelay = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "mysql2es_canal_delay",
			Help: "The canal slave lag",
		},
	)
)

func (r *River) collectMetrics() {
	for range time.Tick(10 * time.Second) {
		canalDelay.Set(float64(r.canal.GetDelay()))
	}
}

func InitStatus(addr string, path string) {
	http.Handle(path, promhttp.Handler())
	http.HandleFunc("/logs", handleMemoryLogs)
	// Used by Admin UI when Docker API restart is unavailable.
	// Process exits; Docker `restart: always` brings the container back.
	http.HandleFunc("/admin/restart", handleSelfRestart)
	http.ListenAndServe(addr, nil)
}

func handleSelfRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"ok":      "restarting",
		"message": "同步进程即将退出并重启Docker 自动拉起",
	})
	go func() {
		time.Sleep(300 * time.Millisecond)
		log.Infof("self-restart requested via /admin/restart, exiting")
		os.Exit(0)
	}()
}
