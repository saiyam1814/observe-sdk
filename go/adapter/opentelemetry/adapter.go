package opentelemetry

import (
	"context"
	"log"
	"time"

	observe "github.com/dylibso/observe-sdk/go"
	trace "go.opentelemetry.io/proto/otlp/trace/v1"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
)

type OTLPProtocol string

const (
	GRPC OTLPProtocol = "grpc"
	HTTP OTLPProtocol = "http/protobuf"
)

type OTelConfig struct {
	ServiceName        string
	EmitTracesInterval time.Duration
	TraceBatchMax      uint32
	Endpoint           string
	Protocol           OTLPProtocol
	ClientHeaders      map[string]string
	AllowInsecure      bool

	client otlptrace.Client
}

type OTelAdapter struct {
	*observe.AdapterBase
	Config *OTelConfig
}

// UseCustomClient accepts a pre-initialized client to allow for customization of how to get data into a collector 
func (a *OTelAdapter) UseCustomClient(client otlptrace.Client) {
	if a.Config != nil {
		a.Config.client = client
	}
}

// NewOTelAdapter will create an instance of an OTelAdapter using the configuration to construct
// an otlptrace.Client based on the Protocol set in the config.
func NewOTelAdapter(config *OTelConfig) *OTelAdapter {
	base := observe.NewAdapterBase(1, 0)

	switch string(config.Protocol) {
	case string(GRPC):
		options := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(config.Endpoint),
			otlptracegrpc.WithTimeout(2 * time.Second),
			otlptracegrpc.WithHeaders(config.ClientHeaders),
		}

		if config.AllowInsecure {
			options = append(options, otlptracegrpc.WithInsecure())
		}
		config.client = otlptracegrpc.NewClient(options...)
	case string(HTTP):
		options := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(config.Endpoint),
			otlptracehttp.WithTimeout(2 * time.Second),
			otlptracehttp.WithHeaders(config.ClientHeaders),
		}

		if config.AllowInsecure {
			options = append(options, otlptracehttp.WithInsecure())
		}
		config.client = otlptracehttp.NewClient(options...)
	}

	adapter := &OTelAdapter{
		AdapterBase: &base,
		Config:      config,
	}

	adapter.AdapterBase.SetFlusher(adapter)

	return adapter
}

func (o *OTelAdapter) Start(ctx context.Context) {
	o.AdapterBase.Start(ctx, o)
	o.Config.client.Start(ctx)
}

func (o *OTelAdapter) StopWithContext(ctx context.Context, wait bool) error {
	o.AdapterBase.Stop(wait)
	return o.Config.client.Stop(ctx)
}

func (o *OTelAdapter) Stop(wait bool) {
	o.AdapterBase.Stop(wait)
	err := o.Config.client.Stop(context.Background())
	if err != nil {
		log.Println("failed to stop otlptrace.Client from wasm sdk")
	}
}

func (o *OTelAdapter) HandleTraceEvent(te observe.TraceEvent) {
	o.AdapterBase.HandleTraceEvent(te)
}

func (o *OTelAdapter) Flush(evts []observe.TraceEvent) error {
	for _, te := range evts {
		traceId := te.TelemetryId.ToHex16()

		var allSpans []*trace.Span
		for _, e := range te.Events {
			switch event := e.(type) {
			case observe.CallEvent: // TODO: consider renaming to FunctionCall for consistency across Rust & JS
				spans := o.MakeOtelCallSpans(event, nil, traceId)
				if len(spans) > 0 {
					allSpans = append(allSpans, spans...)
				}
			case observe.MemoryGrowEvent:
				log.Println("MemoryGrowEvent should be attached to a span")
			case observe.CustomEvent:
				log.Println("opentelemetry adapter does not respect custom events")
			}
		}

		if len(allSpans) == 0 {
			return nil
		}

		t := observe.NewOtelTrace(traceId, o.Config.ServiceName, allSpans)

		if te.AdapterMeta != nil {
			meta, ok := te.AdapterMeta.(map[string]string)
			if ok {
				t.SetMetadata(&te, meta)
			} else {
				log.Println("metadata must be of type map[string]string")
			}
		}

		err := o.Config.client.UploadTraces(context.Background(), t.TracesData.ResourceSpans)
		if err != nil {
			log.Println("failed to upload wasm traces to otel endpoint", err)
		}
	}

	return nil
}
