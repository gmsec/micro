package tracer

import (
	"context"
	"fmt"
	"io"
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

var _jaegerInfo *jaegerInfo

// WithTracer addr:地址，percent 概率采集
func WithTracer(head, addr string, percent int, maxTagValueLength int) {
	if maxTagValueLength <= 0 {
		maxTagValueLength = jaeger.DefaultMaxTagValueLength
	}
	if _jaegerInfo == nil {
		_jaegerInfo = &jaegerInfo{
			addr:              addr,
			head:              head,
			percent:           float64(percent) * 0.01,
			maxTagValueLength: maxTagValueLength,
		}
	}
	_jaegerInfo.addr = addr
	// initTrace()
}

func SetServiceName(service string) {
	if _jaegerInfo == nil {
		_jaegerInfo = &jaegerInfo{
			serviceName: service,
		}
	}
	_jaegerInfo.serviceName = service
	initTrace()
}

func initTrace() {
	serviceName := _jaegerInfo.serviceName
	if len(_jaegerInfo.head) > 0 {
		serviceName = fmt.Sprintf("%v_%v", _jaegerInfo.head, _jaegerInfo.serviceName)
	}
	if len(_jaegerInfo.addr) > 0 && len(serviceName) > 0 {
		jcfg := jaegercfg.Configuration{
			Sampler: &jaegercfg.SamplerConfig{
				Type:  jaeger.SamplerTypeProbabilistic,
				Param: _jaegerInfo.percent,
			},
			ServiceName: serviceName,
		}

		report := &jaegercfg.ReporterConfig{
			LogSpans:            dev.IsDev(),
			BufferFlushInterval: 1 * time.Second,
			LocalAgentHostPort:  _jaegerInfo.addr,
		}

		var err error
		reporter, _ := report.NewReporter(_jaegerInfo.serviceName, jaeger.NewNullMetrics(), jaeger.StdLogger)
		_jaegerInfo.tracer, _jaegerInfo.closer, err = jcfg.NewTracer(
			jaegercfg.Reporter(reporter),
			jaegercfg.MaxTagValueLength(_jaegerInfo.maxTagValueLength),
		)

		if err != nil {
			mylog.Error(err)
		}
	}
}

func GetTracer() opentracing.Tracer {
	if _jaegerInfo == nil {
		return nil
	}

	return _jaegerInfo.tracer
}

func CloseTracer() {
	if _jaegerInfo == nil {
		return
	}

	if _jaegerInfo.closer != nil {
		_jaegerInfo.closer.Close()
	}
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
	if ctx == nil {
		ctx, _ = context.WithTimeout(context.Background(), 3*time.Second)
	}

	_trace := GetTracer()
	if _trace == nil {
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
