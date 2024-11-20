package web

import (
	"context"
	"log"
	"net/http"
	"testing"
	"time"
)

func TestShutdown(t *testing.T) {
	s1 := NewServer("business", "localhost:9080")
	s1.Handle("/", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("hello"))
	}))
	s2 := NewServer("admin", "localhost:9081")
	app := NewApp([]*Server{s1, s2}, WithShutdownCallbacks(StoreCacheToDBCallback))
	app.StartAndServe()
}

func StoreCacheToDBCallback(ctx context.Context) {
	done := make(chan struct{}, 1)
	go func() {
		// 你的业务逻辑，比如说这里我们模拟的是将本地缓存刷新到数据库里面
		// 这里我们简单的睡一段时间来模拟
		log.Printf("刷新缓存中……")
		time.Sleep(1 * time.Second)
	}()
	select {
	case <-ctx.Done():
		log.Printf("刷新缓存超时")
	case <-done:
		log.Printf("缓存被刷新到了 DB")
	}
}
