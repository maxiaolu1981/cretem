package trace

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	uuid "github.com/satori/go.uuid"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/code"
	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/metrics"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
)

// Phase represents the lifecycle phase of a trace payload.
type Phase string

const (
	// PhaseHTTP marks the HTTP request handling phase.
	PhaseHTTP Phase = "http"
	// PhaseAsync marks the asynchronous processing phase (e.g., Kafka consumer).
	PhaseAsync Phase = "async"
)

// Options describes the initialization parameters for a new trace session.
type Options struct {
	TraceID      string
	Service      string
	Component    string
	Operation    string
	Phase        Phase
	Path         string
	Method       string
	ClientIP     string
	RequestID    string
	UserAgent    string
	Host         string
	AwaitTimeout time.Duration
	Now          time.Time
}

// RequestContext captures core request metadata.
type RequestContext struct {
	RequestID string                 `json:"request_id"`
	ClientIP  string                 `json:"client_ip"`
	UserID    string                 `json:"user_id"`
	Operator  string                 `json:"operator"`
	Method    string                 `json:"http_method"`
	Path      string                 `json:"http_path"`
	Status    int                    `json:"http_status"`
	Extra     map[string]interface{} `json:"extra,omitempty"`
}

// BusinessMetrics summarizes overall business outcome for a request.
type BusinessMetrics struct {
	Operation       string                 `json:"operation"`
	TotalDurationMs float64                `json:"total_duration_ms"`
	OverallStatus   string                 `json:"overall_status"`
	BusinessCode    string                 `json:"business_code"`
	BusinessMessage string                 `json:"business_message"`
	Summary         map[string]interface{} `json:"performance_summary,omitempty"`
}

// Span represents a single timed operation within a trace.
type Span struct {
	ID           string                 `json:"span_id"`
	ParentID     string                 `json:"parent_id"`
	Component    string                 `json:"component"`
	Operation    string                 `json:"operation"`
	StartTime    time.Time              `json:"start_time"`
	EndTime      time.Time              `json:"end_time"`
	DurationMs   float64                `json:"duration_ms"`
	Status       string                 `json:"status"`
	BusinessCode string                 `json:"business_code"`
	Details      map[string]interface{} `json:"details,omitempty"`
	Metrics      []string               `json:"prometheus_metrics,omitempty"`
}

// Trace captures spans and metadata for a single logical request or async flow.
type Trace struct {
	ID        string
	Service   string
	Component string
	Operation string
	Phase     Phase

	Start time.Time
	End   time.Time

	RequestContext  RequestContext
	BusinessMetrics BusinessMetrics

	level          string
	status         string
	businessCode   string
	businessMsg    string
	httpStatusCode int

	mu    sync.Mutex
	spans []*Span

	asyncExpected bool
	asyncDeadline time.Time
}

type traceContextKey struct{}

type currentSpanKey struct{}

// Start attaches a new Trace to the provided context and returns the derived context and Trace pointer.
func Start(ctx context.Context, opts Options) (context.Context, *Trace) {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	traceID := opts.TraceID
	if traceID == "" {
		traceID = uuid.Must(uuid.NewV4()).String()
	}

	t := &Trace{
		ID:        traceID,
		Service:   opts.Service,
		Component: opts.Component,
		Operation: opts.Operation,
		Phase:     opts.Phase,
		Start:     opts.Now,
		RequestContext: RequestContext{
			RequestID: nonEmpty(opts.RequestID, traceID),
			ClientIP:  opts.ClientIP,
			Method:    opts.Method,
			Path:      opts.Path,
		},
		BusinessMetrics: BusinessMetrics{
			Operation: opts.Operation,
		},
		level:  "INFO",
		status: "success",
	}

	if opts.AwaitTimeout > 0 {
		t.asyncDeadline = opts.Now.Add(opts.AwaitTimeout)
	}

	ctx = context.WithValue(ctx, traceContextKey{}, t)
	return ctx, t
}

// FromContext retrieves the Trace from context if present.
func FromContext(ctx context.Context) *Trace {
	if ctx == nil {
		return nil
	}
	if v := ctx.Value(traceContextKey{}); v != nil {
		if t, ok := v.(*Trace); ok {
			return t
		}
	}
	return nil
}

// TraceIDFromContext returns the trace identifier if available.
func TraceIDFromContext(ctx context.Context) string {
	if t := FromContext(ctx); t != nil {
		return t.ID
	}
	return ""
}

// StartSpan starts a new span under the current trace.
func StartSpan(ctx context.Context, component, operation string) (context.Context, *Span) {
	t := FromContext(ctx)
	if t == nil {
		return ctx, nil
	}

	spanID := uuid.Must(uuid.NewV4()).String()
	parentID := ""
	if v := ctx.Value(currentSpanKey{}); v != nil {
		if parent, ok := v.(string); ok {
			parentID = parent
		}
	}

	span := &Span{
		ID:        spanID,
		ParentID:  parentID,
		Component: component,
		Operation: operation,
		StartTime: time.Now(),
		Status:    "success",
	}

	t.mu.Lock()
	t.spans = append(t.spans, span)
	t.mu.Unlock()

	spanCtx := context.WithValue(ctx, currentSpanKey{}, spanID)
	return spanCtx, span
}

// StartSpanWithParent starts a new span and explicitly sets the parent span ID when provided.
func StartSpanWithParent(ctx context.Context, component, operation, parentID string) (context.Context, *Span) {
	t := FromContext(ctx)
	if t == nil {
		return ctx, nil
	}

	spanID := uuid.Must(uuid.NewV4()).String()
	if parentID == "" {
		if v := ctx.Value(currentSpanKey{}); v != nil {
			if parent, ok := v.(string); ok {
				parentID = parent
			}
		}
	}

	span := &Span{
		ID:        spanID,
		ParentID:  parentID,
		Component: component,
		Operation: operation,
		StartTime: time.Now(),
		Status:    "success",
	}

	t.mu.Lock()
	t.spans = append(t.spans, span)
	t.mu.Unlock()

	spanCtx := context.WithValue(ctx, currentSpanKey{}, spanID)
	return spanCtx, span
}

// EndSpan finalizes the span with status and details.
func EndSpan(span *Span, status, businessCode string, details map[string]interface{}, metricsLabels ...string) {
	if span == nil {
		return
	}
	span.EndTime = time.Now()
	span.DurationMs = float64(span.EndTime.Sub(span.StartTime)) / float64(time.Millisecond)
	if status != "" {
		span.Status = status
	}
	if businessCode != "" {
		span.BusinessCode = businessCode
	}
	if details != nil {
		span.Details = details
	}
	if len(metricsLabels) > 0 {
		span.Metrics = append(span.Metrics, metricsLabels...)
	}

	metrics.RecordTraceSpanDuration(span.Component, span.Operation, span.Status, span.EndTime.Sub(span.StartTime))
}

// RecordOutcome captures the business outcome for the trace.
func RecordOutcome(ctx context.Context, businessCode, message, status string, httpStatus int) {
	t := FromContext(ctx)
	if t == nil {
		return
	}

	if status == "" {
		status = "success"
	}
	if businessCode == "" {
		businessCode = strconv.Itoa(code.ErrUnknown)
	}

	t.mu.Lock()
	t.status = status
	t.businessCode = businessCode
	t.businessMsg = message
	t.httpStatusCode = httpStatus
	if status == "error" {
		t.level = "ERROR"
	} else if status == "degraded" && t.level != "ERROR" {
		t.level = "WARN"
	}
	t.mu.Unlock()
}

// SetOperator records the operator or user identifier on the trace.
func SetOperator(ctx context.Context, operator string) {
	t := FromContext(ctx)
	if t == nil || operator == "" {
		return
	}
	t.mu.Lock()
	t.RequestContext.Operator = operator
	if t.RequestContext.UserID == "" {
		t.RequestContext.UserID = operator
	}
	t.mu.Unlock()
}

// SetUserID records the acting user id independently.
func SetUserID(ctx context.Context, userID string) {
	t := FromContext(ctx)
	if t == nil || userID == "" {
		return
	}
	t.mu.Lock()
	t.RequestContext.UserID = userID
	t.mu.Unlock()
}

// AddRequestTag attaches arbitrary metadata to the request context for downstream logging.
func AddRequestTag(ctx context.Context, key string, value interface{}) {
	if key == "" {
		return
	}
	t := FromContext(ctx)
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.RequestContext.Extra == nil {
		t.RequestContext.Extra = make(map[string]interface{})
	}
	t.RequestContext.Extra[key] = value
	t.mu.Unlock()
}

// GetRequestTag retrieves a request-scoped tag previously stored on the trace.
func GetRequestTag(ctx context.Context, key string) (interface{}, bool) {
	if key == "" {
		return nil, false
	}
	t := FromContext(ctx)
	if t == nil {
		return nil, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.RequestContext.Extra == nil {
		return nil, false
	}
	v, ok := t.RequestContext.Extra[key]
	return v, ok
}

// UpdateHTTPStatus records the latest HTTP status code and escalates outcome metadata when needed.
func UpdateHTTPStatus(ctx context.Context, httpStatus int) {
	t := FromContext(ctx)
	if t == nil {
		return
	}
	t.mu.Lock()
	t.httpStatusCode = httpStatus
	if t.RequestContext.Status == 0 {
		t.RequestContext.Status = httpStatus
	}
	if httpStatus >= http.StatusInternalServerError {
		t.status = "error"
		t.level = "ERROR"
	} else if httpStatus >= http.StatusBadRequest && t.status != "error" {
		t.status = "degraded"
		if t.level == "INFO" {
			t.level = "WARN"
		}
	}
	t.mu.Unlock()
}

// ExpectAsync marks the trace as expecting asynchronous completion.
func ExpectAsync(ctx context.Context, deadline time.Time) {
	t := FromContext(ctx)
	if t == nil {
		return
	}
	t.mu.Lock()
	t.asyncExpected = true
	if !deadline.IsZero() {
		t.asyncDeadline = deadline
	}
	t.mu.Unlock()
}

// Complete marks the trace phase as complete and forwards to the collector for emission.
func Complete(ctx context.Context) {
	t := FromContext(ctx)
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.End.IsZero() {
		t.End = time.Now()
	}
	t.RequestContext.Status = t.httpStatusCode
	t.BusinessMetrics.BusinessCode = t.businessCode
	t.BusinessMetrics.BusinessMessage = t.businessMsg
	t.BusinessMetrics.OverallStatus = t.status
	t.BusinessMetrics.TotalDurationMs = float64(t.End.Sub(t.Start)) / float64(time.Millisecond)
	t.mu.Unlock()

	defaultCollector.Enqueue(t)
}

// NewDetached creates a trace for asynchronous stages without inheriting an existing context.
func NewDetached(opts Options) (*Trace, context.Context) {
	ctx := context.Background()
	opts.Phase = PhaseAsync
	ctx, t := Start(ctx, opts)
	return t, ctx
}

// MarshalSpans returns ordered spans for logging.
func (t *Trace) MarshalSpans() []*Span {
	t.mu.Lock()
	defer t.mu.Unlock()
	spans := make([]*Span, len(t.spans))
	copy(spans, t.spans)
	sort.Slice(spans, func(i, j int) bool {
		return spans[i].StartTime.Before(spans[j].StartTime)
	})
	return spans
}

// ToLogPayload builds the final log message payload.
func (t *Trace) ToLogPayload(asyncSpans []*Span) map[string]interface{} {
	spans := t.MarshalSpans()
	if len(asyncSpans) > 0 {
		spans = append(spans, asyncSpans...)
		sort.Slice(spans, func(i, j int) bool {
			return spans[i].StartTime.Before(spans[j].StartTime)
		})
	}

	spanPayload := make([]map[string]interface{}, 0, len(spans))
	errorCategories := map[string]int{
		"validation": 0,
		"database":   0,
		"kafka":      0,
		"redis":      0,
		"business":   0,
	}
	degraded := 0
	errors := 0

	errorSpanIDs := make(map[string]struct{})
	errorParentIDs := make(map[string]struct{})
	for _, span := range spans {
		if span.Status == "error" {
			errorSpanIDs[span.ID] = struct{}{}
		}
	}
	for _, span := range spans {
		if span.Status == "error" {
			if _, ok := errorSpanIDs[span.ParentID]; ok {
				errorParentIDs[span.ParentID] = struct{}{}
			}
		}
	}

	for _, span := range spans {
		entry := map[string]interface{}{
			"span_id":     span.ID,
			"parent_id":   span.ParentID,
			"component":   span.Component,
			"operation":   span.Operation,
			"start_time":  span.StartTime.UnixNano() / int64(time.Millisecond),
			"end_time":    span.EndTime.UnixNano() / int64(time.Millisecond),
			"duration_ms": span.DurationMs,
			"status":      span.Status,
		}
		if span.BusinessCode != "" {
			entry["business_code"] = span.BusinessCode
		}
		if len(span.Metrics) > 0 {
			entry["prometheus_metric"] = span.Metrics
		}
		if len(span.Details) > 0 {
			entry["details"] = span.Details
			categorizeError(span, errorCategories)
		}
		spanPayload = append(spanPayload, entry)
		if span.Status == "error" {
			if _, hasErrorChild := errorParentIDs[span.ID]; !hasErrorChild {
				errors++
			}
		} else if span.Status == "degraded" {
			degraded++
		}
	}

	var level = t.level
	if errors > 0 {
		level = "ERROR"
	} else if degraded > 0 && level == "INFO" {
		level = "WARN"
	}

	performance := make(map[string]interface{})
	performance["api_processing_ms"] = estimateComponentDuration(spans, "user-controller") + estimateComponentDuration(spans, "user-service")
	performance["kafka_production_ms"] = estimateComponentDuration(spans, "kafka-producer")
	performance["kafka_consumption_ms"] = estimateComponentDuration(spans, "kafka-consumer")

	payload := map[string]interface{}{
		"trace_id":  t.ID,
		"level":     level,
		"timestamp": t.End.UTC().Format(time.RFC3339Nano),
		"service":   t.Service,
		"component": t.Component,
		"call_chain": map[string]interface{}{
			"root_operation": t.Operation,
			"start_time":     t.Start.UnixNano() / int64(time.Millisecond),
			"end_time":       t.End.UnixNano() / int64(time.Millisecond),
			"spans":          spanPayload,
		},
		"request_context": t.RequestContext,
		"business_metrics": map[string]interface{}{
			"operation":           t.BusinessMetrics.Operation,
			"total_duration_ms":   t.BusinessMetrics.TotalDurationMs,
			"overall_status":      t.BusinessMetrics.OverallStatus,
			"business_code":       t.BusinessMetrics.BusinessCode,
			"business_message":    t.BusinessMetrics.BusinessMessage,
			"performance_summary": performance,
		},
		"error_analysis": map[string]interface{}{
			"total_errors":        errors,
			"degraded_operations": degraded,
			"error_categories":    errorCategories,
		},
	}

	return payload
}

// Log emits the trace payload using structured logging.
func (t *Trace) Log(asyncSpans []*Span) {
	payload := t.ToLogPayload(asyncSpans)
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		log.Errorw("trace marshal failure", "traceID", t.ID, "error", err)
		return
	}
	if t.level == "ERROR" {
		log.Error(string(payloadJSON))
	} else if t.level == "WARN" {
		log.Warn(string(payloadJSON))
	} else {
		log.Info(string(payloadJSON))
	}

	metrics.RecordTraceOperationDuration(t.Operation, string(t.Phase), t.status, t.End.Sub(t.Start))
}

func nonEmpty(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

func categorizeError(span *Span, categories map[string]int) {
	if span == nil || len(span.Details) == 0 {
		return
	}
	if domain, ok := span.Details["error_category"].(string); ok {
		if domain != "" {
			categories[domain]++
		}
	}
}

func estimateComponentDuration(spans []*Span, component string) float64 {
	var total float64
	for _, span := range spans {
		if span.Component == component {
			total += span.DurationMs
		}
	}
	return total
}

// Collector coordinates trace emission across HTTP and async phases.
type Collector struct {
	mu           sync.Mutex
	traces       map[string]*collectorEntry
	awaitTimeout time.Duration
}

type collectorEntry struct {
	httpTrace  *Trace
	asyncSpans []*Span
	asyncReady bool
	timer      *time.Timer
}

var defaultCollector = NewCollector()

// NewCollector constructs a collector with a default timeout.
func NewCollector() *Collector {
	return &Collector{
		traces:       make(map[string]*collectorEntry),
		awaitTimeout: 30 * time.Second,
	}
}

// SetAwaitTimeout configures the waiting timeout.
func (c *Collector) SetAwaitTimeout(timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	c.mu.Lock()
	c.awaitTimeout = timeout
	c.mu.Unlock()
}

// Enqueue registers a completed trace phase.
func (c *Collector) Enqueue(t *Trace) {
	if t == nil {
		return
	}

	if t.Phase == PhaseAsync {
		c.mergeAsync(t)
		return
	}

	c.mergeHTTP(t)
}

func (c *Collector) mergeHTTP(t *Trace) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.traces[t.ID]
	if !exists {
		entry = &collectorEntry{}
		c.traces[t.ID] = entry
	}
	entry.httpTrace = t

	if entry.asyncReady || !t.asyncExpected {
		asyncSpans := entry.asyncSpans
		delete(c.traces, t.ID)
		c.mu.Unlock()
		t.Log(asyncSpans)
		c.mu.Lock()
		return
	}

	// Wait for async completion, schedule timeout
	timeout := c.awaitTimeout
	if !t.asyncDeadline.IsZero() {
		if deadlineDelta := time.Until(t.asyncDeadline); deadlineDelta > 0 {
			timeout = deadlineDelta
		}
	}

	if timeout <= 0 {
		asyncSpans := entry.asyncSpans
		delete(c.traces, t.ID)
		c.mu.Unlock()
		t.Log(asyncSpans)
		c.mu.Lock()
		return
	}

	entry.timer = time.AfterFunc(timeout, func() {
		c.flushTimeout(t.ID)
	})
}

func (c *Collector) mergeAsync(t *Trace) {
	spans := t.MarshalSpans()
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.traces[t.ID]
	if !exists {
		entry = &collectorEntry{}
		c.traces[t.ID] = entry
	}
	entry.asyncSpans = append(entry.asyncSpans, spans...)
	if !t.asyncExpected {
		entry.asyncReady = true
	}

	if entry.httpTrace != nil && entry.asyncReady {
		if entry.timer != nil {
			entry.timer.Stop()
		}
		httpTrace := entry.httpTrace
		async := entry.asyncSpans
		delete(c.traces, t.ID)
		c.mu.Unlock()
		httpTrace.Log(async)
		c.mu.Lock()
	}
}

func (c *Collector) flushTimeout(traceID string) {
	c.mu.Lock()
	entry, exists := c.traces[traceID]
	if !exists {
		c.mu.Unlock()
		return
	}
	if entry.httpTrace == nil {
		delete(c.traces, traceID)
		c.mu.Unlock()
		return
	}

	httpTrace := entry.httpTrace
	async := entry.asyncSpans
	delete(c.traces, traceID)
	c.mu.Unlock()

	httpTrace.Log(async)
}
