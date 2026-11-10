package tracer

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	"github.com/xxjwxc/public/dev"
	"github.com/xxjwxc/public/message"
	"github.com/xxjwxc/public/mylog"
)

type jaegerInfo struct {
	addr              string
	serviceName       string
	percent           float64
	head              string
	maxTagValueLength int
	tracer            opentracing.Tracer
	closer            io.Closer
}

var (
	// jaegerMu 保护 _jaegerInfo 的全部读写。
	jaegerMu sync.Mutex
	// initOnce 保证 _jaegerInfo 只被创建一次,避免 WithTracer / SetServiceName
	// 并发时各自 new 一个对象互相覆盖。
	initOnce sync.Once

	_jaegerInfo *jaegerInfo
)

// ensureInfo 返回全局唯一的 jaegerInfo(调用方须持有 jaegerMu)。
func ensureInfo() *jaegerInfo {
	initOnce.Do(func() {
		_jaegerInfo = &jaegerInfo{}
	})
	return _jaegerInfo
}

// WithTracer 配置 jaeger agent 地址等参数。
// 配置齐备(serviceName + addr)时初始化 tracer;重复调用会先关掉旧 closer 再重建,
// 避免后台 reporter goroutine 与 UDP 连接泄漏。
func WithTracer(head, addr string, percent int, maxTagValueLength int) {
	if maxTagValueLength <= 0 {
		maxTagValueLength = jaeger.DefaultMaxTagValueLength
	}

	jaegerMu.Lock()
	defer jaegerMu.Unlock()

	info := ensureInfo()
	info.addr = addr
	info.head = head
	info.percent = float64(percent) * 0.01
	info.maxTagValueLength = maxTagValueLength

	if err := info.rebuild(); err != nil {
		mylog.Error(err)
	}
}

// SetServiceName 设置服务名。重复传入相同名字且 tracer 已就绪时只更新字段,
// 不再重建 tracer;名字变化则先关闭旧 closer 再重建。
func SetServiceName(service string) {
	jaegerMu.Lock()
	defer jaegerMu.Unlock()

	info := ensureInfo()
	if info.serviceName == service && info.tracer != nil {
		return
	}
	info.serviceName = service
	if err := info.rebuild(); err != nil {
		mylog.Error(err)
	}
}

// rebuild 关掉旧的 tracer/reporter 后重新初始化(调用方须持有 jaegerMu)。
func (j *jaegerInfo) rebuild() error {
	if j.closer != nil {
		if err := j.closer.Close(); err != nil {
			mylog.Error(err)
		}
		j.closer = nil
	}
	j.tracer = nil
	return j.initTrace()
}

// initTrace 初始化 tracer。只有 reporter 与 tracer 都创建成功才会写入 j.tracer,
// 任何一步失败都保持"未初始化"状态,保证 GetTracer() 不返回半初始化对象。
func (j *jaegerInfo) initTrace() error {
	if len(j.addr) == 0 || len(j.serviceName) == 0 {
		return nil // 配置未齐,保持未初始化
	}

	serviceName := j.serviceName
	if len(j.head) > 0 {
		serviceName = fmt.Sprintf("%v_%v", j.head, j.serviceName)
	}

	jcfg := jaegercfg.Configuration{
		Sampler: &jaegercfg.SamplerConfig{
			Type:  jaeger.SamplerTypeProbabilistic,
			Param: j.percent,
		},
		ServiceName: serviceName,
	}

	report := &jaegercfg.ReporterConfig{
		LogSpans:            dev.IsDev(),
		BufferFlushInterval: 1 * time.Second,
		LocalAgentHostPort:  j.addr,
	}

	reporter, err := report.NewReporter(j.serviceName, jaeger.NewNullMetrics(), jaeger.StdLogger)
	if err != nil {
		return fmt.Errorf("jaeger: create reporter: %w", err)
	}

	maxTag := j.maxTagValueLength
	if maxTag <= 0 {
		maxTag = jaeger.DefaultMaxTagValueLength
	}
	tracer, closer, err := jcfg.NewTracer(
		jaegercfg.Reporter(reporter),
		jaegercfg.MaxTagValueLength(maxTag),
	)
	if err != nil {
		// NewTracer 失败时 reporter 可能已经起了后台 goroutine,主动关掉。
		reporter.Close()
		return fmt.Errorf("jaeger: create tracer: %w", err)
	}

	j.tracer, j.closer = tracer, closer
	return nil
}

// GetTracer 返回已就绪的 tracer;未初始化或初始化失败时返回 nil。
func GetTracer() opentracing.Tracer {
	jaegerMu.Lock()
	defer jaegerMu.Unlock()

	if _jaegerInfo == nil {
		return nil
	}
	return _jaegerInfo.tracer
}

// CloseTracer 关闭全局 tracer,幂等:多次调用不会重复 Close 已释放的 closer。
// 由 micro.Service 在最后一个服务停止时统一调用,单个服务 Stop 不应调用它。
func CloseTracer() {
	jaegerMu.Lock()
	defer jaegerMu.Unlock()

	if _jaegerInfo == nil {
		return
	}
	if _jaegerInfo.closer != nil {
		if err := _jaegerInfo.closer.Close(); err != nil {
			mylog.Error(err)
		}
		_jaegerInfo.closer = nil
	}
	_jaegerInfo.tracer = nil
}

type SpanOption func(span opentracing.Span)

func SpanWithError(err error) SpanOption {
	return func(span opentracing.Span) {
		if err != nil {
			ext.Error.Set(span, true)
			span.LogFields(log.String("event", "error"), log.String("msg", err.Error()))
		}
	}
}

// example:
// SpanWithLog(
//
//	"event", "soft error",
//	"type", "cache timeout",
//	"waited.millis", 1500)
func SpanWithLog(arg ...interface{}) SpanOption {
	return func(span opentracing.Span) {
		span.LogKV(arg...)
	}
}

func Start(spanName string, ctx context.Context) (newCtx context.Context, finish func(...SpanOption), err error) {
	var cancel context.CancelFunc
	if ctx == nil {
		ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	}

	_trace := GetTracer()
	if _trace == nil {
		if cancel != nil {
			cancel()
		}
		err = message.GetError(message.InvalidArgument)
		return
	}

	span, newCtx := opentracing.StartSpanFromContextWithTracer(ctx, _trace, spanName,
		opentracing.Tag{Key: string(ext.Component), Value: "func"},
	)

	finish = func(ops ...SpanOption) {
		for _, o := range ops {
			o(span)
		}
		span.Finish()
		if cancel != nil { // 释放 Start 内部创建的 ctx 超时定时器
			cancel()
		}
	}

	return
}

/**** 在业务代码中使用

有时候只监控一个"api"是不够的，还需要监控到程序中的代码片段(如方法)，可以这样封装一个方法
使用

newCtx, finish := tracer.Start("DoSomeThing", ctx)
err := DoSomeThing(newCtx)
finish(tracer.SpanWithError(err))
if err != nil{
  ...
}
***/
