package main

import (
	"flag"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/juju/errors"
	"github.com/siddontang/go-log/log"
	"github.com/JasonYe88/go-mysqlbinlog-to-es/river"
)

// 全局日志写入器，用于程序退出时关闭
var globalLogWriter *river.DailyLogWriter

var configFile = flag.String("config", "./configs/river.toml", "go-mysqlbinlog-to-es config file")
var my_addr = flag.String("my_addr", "", "MySQL addr")
var my_user = flag.String("my_user", "", "MySQL user")
var my_pass = flag.String("my_pass", "", "MySQL password")
var es_addr = flag.String("es_addr", "", "Elasticsearch addr")
var data_dir = flag.String("data_dir", "", "path for go-mysqlbinlog-to-es to save data")
var server_id = flag.Int("server_id", 0, "MySQL server id, as a pseudo slave")
var flavor = flag.String("flavor", "", "flavor: mysql or mariadb")
var execution = flag.String("exec", "", "mysqldump execution path")
var logLevel = flag.String("log_level", "info", "log level")
var logDir = flag.String("log_dir", "logs", "log directory")
var logMaxDays = flag.Int("log_max_days", 7, "max days to keep logs, 0 means no cleanup")

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.Parse()

	setupLogRing(*logLevel)

	sc := make(chan os.Signal, 1)
	signal.Notify(sc,
		os.Kill,
		os.Interrupt,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	cfg, err := river.NewConfigWithFile(*configFile)
	if err != nil {
		println(errors.ErrorStack(err))
		return
	}

	if len(*my_addr) > 0 {
		cfg.MyAddr = *my_addr
	}

	if len(*my_user) > 0 {
		cfg.MyUser = *my_user
	}

	if len(*my_pass) > 0 {
		cfg.MyPassword = *my_pass
	}

	if *server_id > 0 {
		cfg.ServerID = uint32(*server_id)
	}

	if len(*es_addr) > 0 {
		cfg.ESAddr = *es_addr
	}

	if len(*data_dir) > 0 {
		cfg.DataDir = *data_dir
	}

	if len(*flavor) > 0 {
		cfg.Flavor = *flavor
	}

	if len(*execution) > 0 {
		cfg.DumpExec = *execution
	}

	r, err := river.NewRiver(cfg)
	if err != nil {
		println(errors.ErrorStack(err))
		return
	}

	done := make(chan struct{}, 1)
	go func() {
		r.Run()
		done <- struct{}{}
	}()

	select {
	case n := <-sc:
		log.Infof("receive signal %v, closing", n)
	case <-r.Ctx().Done():
		log.Infof("context is done with %v, closing", r.Ctx().Err())
	}

	r.Close()
	<-done

	// 关闭日志写入器
	if globalLogWriter != nil {
		globalLogWriter.Close()
	}
}

func setupLogRing(level string) {
	var writers []io.Writer
	writers = append(writers, os.Stdout, river.GlobalLogRing)

	// 创建按天分割的日志文件
	logWriter, err := river.NewDailyLogWriter(*logDir, "sync", *logMaxDays)
	if err != nil {
		log.Warnf("create daily log writer failed: %v, only output to stdout", err)
	} else {
		globalLogWriter = logWriter
		writers = append(writers, logWriter)
	}

	mw := io.MultiWriter(writers...)
	h, err := log.NewStreamHandler(mw)
	if err != nil {
		log.SetLevelByName(level)
		return
	}
	l := log.NewDefault(h)
	l.SetLevelByName(level)
	log.SetDefaultLogger(l)
}
