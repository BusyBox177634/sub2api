package handler

import (
	"bytes"
	"net/http"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type usageDetailCaptureWriter struct {
	gin.ResponseWriter
	totalBytes int
	buf        bytes.Buffer
}

var usageDetailCaptureWriterPool = sync.Pool{
	New: func() any {
		return &usageDetailCaptureWriter{}
	},
}

func acquireUsageDetailCaptureWriter(rw gin.ResponseWriter) *usageDetailCaptureWriter {
	writer, ok := usageDetailCaptureWriterPool.Get().(*usageDetailCaptureWriter)
	if !ok || writer == nil {
		writer = &usageDetailCaptureWriter{}
	}
	writer.ResponseWriter = rw
	writer.totalBytes = 0
	writer.buf.Reset()
	return writer
}

func releaseUsageDetailCaptureWriter(writer *usageDetailCaptureWriter) {
	if writer == nil {
		return
	}
	writer.ResponseWriter = nil
	writer.totalBytes = 0
	writer.buf.Reset()
	usageDetailCaptureWriterPool.Put(writer)
}

func (w *usageDetailCaptureWriter) Write(b []byte) (int, error) {
	w.captureBytes(b)
	return w.ResponseWriter.Write(b)
}

func (w *usageDetailCaptureWriter) WriteString(s string) (int, error) {
	w.captureBytes([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

func (w *usageDetailCaptureWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
		return
	}
	w.ResponseWriter.Flush()
}

func (w *usageDetailCaptureWriter) captureBytes(b []byte) {
	if w == nil || len(b) == 0 {
		return
	}
	w.totalBytes += len(b)
	_, _ = w.buf.Write(b)
}

func (w *usageDetailCaptureWriter) snapshot() ([]byte, int, bool) {
	if w == nil {
		return nil, 0, false
	}
	payload := make([]byte, w.buf.Len())
	copy(payload, w.buf.Bytes())
	return payload, w.totalBytes, false
}

type usageDetailCaptureState struct {
	originalResponseWriter gin.ResponseWriter
	writer                 *usageDetailCaptureWriter
	requestBody            []byte
	responseFormat         service.UsageLogDetailResponseFormat
	overrideResponseBody   []byte
	overrideResponseBytes  int
	overrideTruncated      bool
}

func beginUsageDetailCapture(
	c *gin.Context,
	requestBody []byte,
	responseFormat service.UsageLogDetailResponseFormat,
) *usageDetailCaptureState {
	if c == nil {
		return nil
	}
	writer := acquireUsageDetailCaptureWriter(c.Writer)
	state := &usageDetailCaptureState{
		originalResponseWriter: c.Writer,
		writer:                 writer,
		requestBody:            cloneUsageDetailBytes(requestBody),
		responseFormat:         responseFormat,
	}
	c.Writer = writer
	return state
}

func (s *usageDetailCaptureState) OverrideResponsePayload(
	responseBody []byte,
	responseBytes int,
	responseTruncated bool,
	responseFormat service.UsageLogDetailResponseFormat,
) {
	if s == nil {
		return
	}
	s.overrideResponseBody = cloneUsageDetailBytes(responseBody)
	s.overrideResponseBytes = responseBytes
	s.overrideTruncated = responseTruncated
	s.responseFormat = responseFormat
}

func (s *usageDetailCaptureState) Build() *service.UsageLogDetailCapture {
	if s == nil {
		return nil
	}
	if len(s.overrideResponseBody) > 0 || s.overrideResponseBytes > 0 || s.overrideTruncated {
		return buildUsageDetailCapture(
			s.requestBody,
			s.overrideResponseBody,
			s.overrideResponseBytes,
			s.overrideTruncated,
			s.responseFormat,
		)
	}
	return buildUsageDetailCaptureFromWriter(s.requestBody, s.writer, s.responseFormat)
}

func (s *usageDetailCaptureState) Close(c *gin.Context) {
	if s == nil {
		return
	}
	if c != nil && s.originalResponseWriter != nil {
		c.Writer = s.originalResponseWriter
	}
	releaseUsageDetailCaptureWriter(s.writer)
}

func usageDetailResponseFormatFromStream(stream bool) service.UsageLogDetailResponseFormat {
	if stream {
		return service.UsageLogDetailResponseFormatSSE
	}
	return service.UsageLogDetailResponseFormatJSON
}

func captureForwardUsageDetail(
	c *gin.Context,
	requestBody []byte,
	stream bool,
	forward func() (*service.ForwardResult, error),
) (*service.ForwardResult, *service.UsageLogDetailCapture, error) {
	state := beginUsageDetailCapture(c, requestBody, usageDetailResponseFormatFromStream(stream))
	defer state.Close(c)
	result, err := forward()
	return result, state.Build(), err
}

func captureOpenAIForwardUsageDetail(
	c *gin.Context,
	requestBody []byte,
	stream bool,
	forward func() (*service.OpenAIForwardResult, error),
) (*service.OpenAIForwardResult, *service.UsageLogDetailCapture, error) {
	state := beginUsageDetailCapture(c, requestBody, usageDetailResponseFormatFromStream(stream))
	defer state.Close(c)
	result, err := forward()
	return result, state.Build(), err
}

func buildUsageDetailCapture(
	requestBody []byte,
	responseBody []byte,
	responseBytes int,
	responseTruncated bool,
	responseFormat service.UsageLogDetailResponseFormat,
) *service.UsageLogDetailCapture {
	if len(requestBody) == 0 && len(responseBody) == 0 && responseBytes == 0 && !responseTruncated {
		return nil
	}
	if responseBytes == 0 && len(responseBody) > 0 {
		responseBytes = len(responseBody)
	}
	return &service.UsageLogDetailCapture{
		RequestBody:              cloneUsageDetailBytes(requestBody),
		ResponseBody:             cloneUsageDetailBytes(responseBody),
		ResponseBodyBytes:        responseBytes,
		ResponseCaptureTruncated: responseTruncated,
		ResponseFormat:           responseFormat,
	}
}

func buildUsageDetailCaptureFromWriter(
	requestBody []byte,
	writer *usageDetailCaptureWriter,
	responseFormat service.UsageLogDetailResponseFormat,
) *service.UsageLogDetailCapture {
	if len(requestBody) == 0 && writer == nil {
		return nil
	}
	responseBody, responseBytes, responseTruncated := writer.snapshot()
	return buildUsageDetailCapture(requestBody, responseBody, responseBytes, responseTruncated, responseFormat)
}

func cloneUsageDetailBytes(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return cloned
}
