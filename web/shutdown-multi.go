package web

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"
)

type Option func(*App)

// ShutdownCallback 优雅退出回调函数
type ShutdownCallback func(ctx context.Context)

func WithShutdownCallbacks(cbs ...ShutdownCallback) Option {
	return func(app *App) {
		app.cbs = cbs
	}
}

type App struct {
	servers []*Server

	// 优雅退出整个超时时间，默认30秒
	shutdownTimeout time.Duration

	// 优雅退出时候等待处理已有请求时间，默认10秒钟
	waitTime time.Duration
	// 自定义回调超时时间，默认三秒钟
	cbTimeout time.Duration

	cbs []ShutdownCallback
}

func NewApp(servers []*Server, opts ...Option) *App {
	res := &App{
		waitTime:        10 * time.Second,
		cbTimeout:       3 * time.Second,
		shutdownTimeout: 30 * time.Second,
		servers:         servers,
	}
	for _, opt := range opts {
		opt(res)
	}

	return res
}

func (a *App) StartAndServe() {
	// 启动所有服务器
	for _, s := range a.servers {
		srv := s
		go func() {
			if err := srv.Start(); err != nil {
				log.Printf("服务器%s已关闭", srv.name)
			} else {
				log.Printf("服务器%s异常退出", srv.name)
			}
		}()
	}
	// 定义要监听的目标信号 signals []os.Signal
	// 调用 signal
	// 当接收到一个退出信号后，会启动后面的 goroutine以及执行 a.web()
	// goroutine 会监听第二个信号，如果超时则强制退出，或者再次接收到信号退出
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, signals...)
	<-ch
	go func() {
		select {
		case <-ch:
			log.Println("强制退出")
			os.Exit(1)
		case <-time.After(a.shutdownTimeout):
			log.Println("超时强制退出")
			os.Exit(1)
		}
	}()
	// 优雅退出
	a.shutdown()
}

func (a *App) shutdown() {
	log.Println("开始关闭应用，停止接收新请求")
	for _, s := range a.servers {
		// 停止接收新请求
		s.rejectReq()
	}
	log.Println("等待正在执行请求完结")
	// 这里可以改造为实时统计正在处理的请求数量，为0 则下一步
	time.Sleep(a.waitTime)

	log.Println("开始关闭服务器")
	// 采用并发关闭所有服务器
	var wg sync.WaitGroup
	wg.Add(len(a.servers))
	for _, srv := range a.servers {
		srvCp := srv
		go func() {
			if err := srvCp.stop(); err != nil {
				log.Printf("关闭服务失败%s \n", srvCp.name)
			}
			wg.Done()
		}()
	}
	wg.Wait()

	log.Println("开始执行自定义回调")
	// 执行回调
	wg.Add(len(a.cbs))
	for _, cb := range a.cbs {
		c := cb
		go func() {
			// 控制回调超时
			ctx, cancel := context.WithTimeout(context.Background(), a.cbTimeout)
			c(ctx)
			cancel()
			wg.Done()
		}()
	}
	wg.Wait()
	log.Println("应用关闭完成")
	a.close()
}

func (a *App) close() {
	// 在这里释放掉一些可能的资源
	time.Sleep(time.Second)
	log.Println("应用关闭")
}

type Server struct {
	srv  *http.Server
	name string
	mux  *serverMux
}

type serverMux struct {
	reject bool
	*http.ServeMux
}

func NewServer(name string, addr string) *Server {
	mux := &serverMux{ServeMux: http.NewServeMux()}
	return &Server{
		name: name,
		mux:  mux,
		srv: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}
}

func (s *serverMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.reject {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("服务已关闭"))
		return
	}
	s.ServeMux.ServeHTTP(w, r)
}

func (s *Server) Handle(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
}

func (s *Server) rejectReq() {
	s.mux.reject = true
}

func (s *Server) Start() error {
	return s.srv.ListenAndServe()
}

func (s *Server) stop() error {
	log.Printf("服务器%s关闭中", s.name)
	return s.srv.Shutdown(context.Background())
}
