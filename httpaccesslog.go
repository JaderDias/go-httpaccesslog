package httpaccesslog

import (
	"fmt"
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

type LogResponseWriter struct {
	http.ResponseWriter
	Stats *responseStats
}

func (this LogResponseWriter) Write(responseBody []byte) (int, error) {
	this.Stats.bodyBytes = len(responseBody)
	return this.ResponseWriter.Write(responseBody)
}

func (this LogResponseWriter) WriteHeader(statusCode int) {
	this.Stats.statusCode = statusCode
	this.ResponseWriter.WriteHeader(statusCode)
}

func Handler(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		stats := responseStats{0, 200}
		logResponseWriter := LogResponseWriter{w, &stats}
		handler(logResponseWriter, r)
		requestDuration := time.Now().Sub(startTime)
		log.Println(formatAccessLog(r, time.Now(), stats, requestDuration, requestDuration, 0))
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
