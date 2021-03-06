package httpaccesslog

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"testing"
	"time"
)

type blackHole struct {
}

func (this blackHole) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (this blackHole) WriteHeader(int) {
}

func (this blackHole) Header() http.Header {
	return nil
}

var usageMessage = []byte(`
supported requests:
	/render/?target=
	/metrics/find/?query=
	/info/?target=
`)

func usageHandler(w http.ResponseWriter, r *http.Request) {
	w.Write(usageMessage)
}

func deniedHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(401)
}

func delayedHandler(w http.ResponseWriter, r *http.Request) {
	time.Sleep(50 * time.Millisecond)
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(404)
}

type clockMock struct {
	time.Time
}

func (this *clockMock) now() time.Time {
	startTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	if this.Time.IsZero() {
		this.Time = time.Now()
		return startTime
	}

	difference := time.Now().Sub(this.Time)
	return startTime.Add(difference)
}

func TestConstructor(t *testing.T) {
	New(os.Stdout)
}

func TestServeMux(t *testing.T) {
	target := &bytes.Buffer{}
	log.SetOutput(target)
	log.SetFlags(0)
	accessLogger := AccessLogger{nil, &clockMock{}}
	http.HandleFunc("/", accessLogger.Handle(notFoundHandler))
	go http.ListenAndServe(":5000", nil)
	tests := []struct {
		path     string
		handler  http.HandlerFunc
		expected string
	}{
		{
			"/NotFound",
			nil,
			"127.0.0.1 - user [10/Nov/2009:23:00:00 +0000] \"GET /NotFound HTTP/1.1\" 404 0 0.000/0.000 \"-\" \"-\" - -\n",
		},
		{
			"/usage",
			usageHandler,
			"127.0.0.1 - user [10/Nov/2009:23:00:00 +0000] \"GET /usage HTTP/1.1\" 200 78 0.000/0.000 \"-\" \"-\" - -\n",
		},
		{
			"/usage/subpath",
			nil,
			// the higher level handler ("/", notFoundHandler) has precedence over ("/usage", usageHandler)
			"127.0.0.1 - user [10/Nov/2009:23:00:00 +0000] \"GET /usage/subpath HTTP/1.1\" 404 0 0.000/0.000 \"-\" \"-\" - -\n",
		},
		{
			"/denied",
			deniedHandler,
			"127.0.0.1 - user [10/Nov/2009:23:00:00 +0000] \"GET /denied HTTP/1.1\" 401 0 0.000/0.000 \"-\" \"-\" - -\n",
		},
		{
			"/delayed",
			delayedHandler,
			"127.0.0.1 - user [10/Nov/2009:23:00:00 +0000] \"GET /delayed HTTP/1.1\" 200 0 0.050/0.050 \"-\" \"-\" - -\n",
		},
	}
	for _, tt := range tests {
		target.Reset()
		if tt.handler != nil {
			http.HandleFunc(tt.path, accessLogger.Handle(tt.handler))
		}

		http.Get("http://user:pass@localhost:5000" + tt.path)
		actual := target.String()
		if actual != tt.expected {
			t.Errorf("\nactual\n%s\nexpected\n%s", actual, tt.expected)
		}
	}
}

func TestHandle(t *testing.T) {
	target := &bytes.Buffer{}
	accessLogger := AccessLogger{log.New(target, "", 0), &clockMock{}}
	tests := []struct {
		remoteAddr string
		username   string
		method     string
		path       string
		proto      string
		referer    string
		userAgent  string
		handler    http.HandlerFunc
		expected   string
	}{
		{
			"127.0.0.1:1234",
			"frank",
			"GET",
			"/apache_pb.gif",
			"HTTP/1.0",
			"http://www.example.com/start.html",
			"Mozilla/4.08 [en] (Win98; I ;Nav)",
			usageHandler,
			"127.0.0.1 - frank [10/Nov/2009:23:00:00 +0000] \"GET /apache_pb.gif HTTP/1.0\" 200 78 0.000/0.000 \"http://www.example.com/start.html\" \"Mozilla/4.08 [en] (Win98; I ;Nav)\" - -\n",
		},
		{
			"10.1.2.254:4567",
			"",
			"GET",
			"/",
			"HTTP/1.1",
			"",
			"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko)",
			deniedHandler,
			"10.1.2.254 - - [10/Nov/2009:23:00:00 +0000] \"GET / HTTP/1.1\" 401 0 0.000/0.000 \"-\" \"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko)\" - -\n",
		},
		{
			"10.1.2.254",
			"",
			"GET",
			"/somepage",
			"HTTP/1.1",
			"https://github.com/",
			"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko)",
			delayedHandler,
			"10.1.2.254 - - [10/Nov/2009:23:00:00 +0000] \"GET /somepage HTTP/1.1\" 200 0 0.050/0.050 \"https://github.com/\" \"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko)\" - -\n",
		},
	}

	for _, tt := range tests {
		request, err := http.NewRequest(tt.method, tt.path, nil)
		if err != nil {
			t.Error("failed to create request")
		}

		request.Proto = tt.proto
		request.RemoteAddr = tt.remoteAddr
		request.SetBasicAuth(tt.username, "")
		if tt.referer != "" {
			request.Header["Referer"] = []string{tt.referer}
		}
		if tt.userAgent != "" {
			request.Header["UserAgent"] = []string{tt.userAgent}
		}
		target.Reset()
		accessLogger.Handle(tt.handler)(blackHole{}, request)
		actual := target.String()
		if actual != tt.expected {
			t.Errorf("\nactual\n%s\nexpected\n%s", actual, tt.expected)
		}
	}
}

func TestFormatAccessLog(t *testing.T) {
	mst, err := time.LoadLocation("MST")
	if err != nil {
		t.Error("Error loading timezone MST")
	}

	cet, err := time.LoadLocation("CET")
	if err != nil {
		t.Error("Error loading timezone CET")
	}

	tests := []struct {
		remoteAddr        string
		username          string
		dateTime          time.Time
		method            string
		path              string
		proto             string
		status            int
		responseBodyBytes int
		requestTime       time.Duration
		upstreamTime      time.Duration
		referer           string
		userAgent         string
		compressionRatio  float64
		expected          string
	}{
		{
			"127.0.0.1:1234",
			"frank",
			time.Date(2000, time.October, 10, 13, 55, 36, 0, mst),
			"GET",
			"/apache_pb.gif",
			"HTTP/1.0",
			http.StatusOK,
			2326,
			0,
			0,
			"http://www.example.com/start.html",
			"Mozilla/4.08 [en] (Win98; I ;Nav)",
			0,
			"127.0.0.1 - frank [10/Oct/2000:13:55:36 -0700] \"GET /apache_pb.gif HTTP/1.0\" 200 2326 0.000/0.000 \"http://www.example.com/start.html\" \"Mozilla/4.08 [en] (Win98; I ;Nav)\" - -",
		},
		{
			"10.1.2.254:4567",
			"",
			time.Date(2016, time.June, 13, 15, 19, 37, 0, cet),
			"GET",
			"/",
			"HTTP/1.1",
			http.StatusUnauthorized,
			22,
			80 * time.Millisecond,
			80 * time.Millisecond,
			"",
			"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko)",
			0,
			"10.1.2.254 - - [13/Jun/2016:15:19:37 +0200] \"GET / HTTP/1.1\" 401 22 0.080/0.080 \"-\" \"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko)\" - -",
		},
		{
			"10.1.2.254",
			"",
			time.Date(2016, time.June, 13, 15, 19, 37, 0, cet),
			"GET",
			"/somepage",
			"HTTP/1.1",
			http.StatusOK,
			950,
			50 * time.Millisecond,
			50 * time.Millisecond,
			"https://github.com/",
			"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko)",
			3.02,
			"10.1.2.254 - - [13/Jun/2016:15:19:37 +0200] \"GET /somepage HTTP/1.1\" 200 950 0.050/0.050 \"https://github.com/\" \"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko)\" 3.02 -",
		},
	}

	for _, tt := range tests {
		request, err := http.NewRequest(tt.method, tt.path, nil)
		if err != nil {
			t.Error("failed to create request")
		}

		request.Proto = tt.proto
		request.RemoteAddr = tt.remoteAddr
		request.SetBasicAuth(tt.username, "")
		if tt.referer != "" {
			request.Header["Referer"] = []string{tt.referer}
		}
		if tt.userAgent != "" {
			request.Header["UserAgent"] = []string{tt.userAgent}
		}
		actual := formatAccessLog(request, tt.dateTime, responseStats{tt.responseBodyBytes, tt.status}, tt.requestTime, tt.upstreamTime, tt.compressionRatio)
		if actual != tt.expected {
			t.Errorf("\nactual\n%s\nexpected\n%s", actual, tt.expected)
		}
	}
}
