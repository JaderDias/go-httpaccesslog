package httpaccesslog

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type responseStats struct {
	bodyBytes  int
	statusCode int
}

type logResponseWriter struct {
	http.ResponseWriter
	stats *responseStats
}

func (this logResponseWriter) Write(responseBody []byte) (int, error) {
	this.stats.bodyBytes = len(responseBody)
	return this.ResponseWriter.Write(responseBody)
}

func (this logResponseWriter) WriteHeader(statusCode int) {
	this.stats.statusCode = statusCode
	this.ResponseWriter.WriteHeader(statusCode)
}

type clock interface {
	now() time.Time
}

type AccessLogger struct {
	*log.Logger
	clock
}

func New(output io.Writer) AccessLogger {
	logger := log.New(output, "", 0)
	return AccessLogger{logger, nil}
}

func (this AccessLogger) now() time.Time {
	if this.clock == nil {
		return time.Now()
	}

	return this.clock.now()
}

func (this AccessLogger) Handle(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		startTime := this.now()
		stats := responseStats{0, 200}
		logWriter := logResponseWriter{w, &stats}
		handler(logWriter, r)
		requestDuration := this.now().Sub(startTime)
		accessLog := formatAccessLog(r, startTime, stats, requestDuration, requestDuration, 0)
		if this.Logger == nil {
			log.Println(accessLog)
		} else {
			this.Logger.Println(accessLog)
		}
	}
}

func formatAccessLog(
	r *http.Request,
	dateTime time.Time,
	stats responseStats,
	requestTime, upstreamTime time.Duration,
	compressionRatio float64) string {
	username, _, ok := r.BasicAuth()
	if !ok || username == "" {
		username = "-"
	}
	referer := "-"
	userAgent := "-"
	if len(r.Header["Referer"]) > 0 {
		referer = r.Header["Referer"][0]
	}
	if len(r.Header["UserAgent"]) > 0 {
		userAgent = r.Header["UserAgent"][0]
	}

	remoteAddr := strings.Split(r.RemoteAddr, ":")
	compressionRatioStr := "-"
	if compressionRatio > 0 {
		compressionRatioStr = strconv.FormatFloat(compressionRatio, 'f', 2, 64)
	}
	requestSeconds := strconv.FormatFloat(float64(requestTime)/float64(time.Second), 'f', 3, 64)
	upstreamSeconds := strconv.FormatFloat(float64(upstreamTime)/float64(time.Second), 'f', 3, 64)
	return fmt.Sprintf(
		"%s - %s [%s] \"%s %s %s\" %d %d %s/%s \"%s\" \"%s\" %s -",
		remoteAddr[0],
		username,
		dateTime.Format("02/Jan/2006:15:04:05 -0700"),
		r.Method,
		r.URL,
		r.Proto,
		stats.statusCode,
		stats.bodyBytes,
		requestSeconds,
		upstreamSeconds,
		referer,
		userAgent,
		compressionRatioStr,
	)
}
