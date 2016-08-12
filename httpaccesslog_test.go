package httpaccesslog

import (
	"fmt"
	"log"
	"net/http"
	"testing"
	"time"
)

type stringContainer struct {
	string
}

type byteWriter struct {
	target *stringContainer
}

func (this byteWriter) Write(bytes []byte) (n int, err error) {
	this.target.string += string(bytes)
	return len(bytes), nil
}

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

func TestServeMux(t *testing.T) {
	target := &stringContainer{""}
	logWriter := byteWriter{target}
	accessLogger := AccessLogger{log.New(logWriter, "", 0)}
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
			"127.0.0.1 - user [%s] \"GET /NotFound HTTP/1.1\" 404 0 0.000/0.000 \"-\" \"-\" - -\n",
		},
		{
			"/usage",
			usageHandler,
			"127.0.0.1 - user [%s] \"GET /usage HTTP/1.1\" 200 78 0.000/0.000 \"-\" \"-\" - -\n",
		},
		{
			"/denied",
			deniedHandler,
			"127.0.0.1 - user [%s] \"GET /denied HTTP/1.1\" 401 0 0.000/0.000 \"-\" \"-\" - -\n",
		},
		{
			"/delayed",
			delayedHandler,
			"127.0.0.1 - user [%s] \"GET /delayed HTTP/1.1\" 200 0 0.050/0.050 \"-\" \"-\" - -\n",
		},
	}
	for _, tt := range tests {
		*target = stringContainer{""}
		if tt.handler != nil {
			http.HandleFunc(tt.path, accessLogger.Handle(tt.handler))
		}

		expected := fmt.Sprintf(tt.expected, time.Now().Format("02/Jan/2006:15:04:05 -0700"))
		http.Get("http://user:pass@localhost:5000" + tt.path)
		actual := target.string
		if actual != expected {
			t.Errorf("\nactual\n%s\nexpected\n%s", actual, expected)
		}
	}
}

func TestHandle(t *testing.T) {
	target := &stringContainer{""}
	logWriter := byteWriter{target}
	accessLogger := AccessLogger{log.New(logWriter, "", 0)}
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
			"127.0.0.1 - frank [%s] \"GET /apache_pb.gif HTTP/1.0\" 200 78 0.000/0.000 \"http://www.example.com/start.html\" \"Mozilla/4.08 [en] (Win98; I ;Nav)\" - -\n",
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
			"10.1.2.254 - - [%s] \"GET / HTTP/1.1\" 401 0 0.000/0.000 \"-\" \"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko)\" - -\n",
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
			"10.1.2.254 - - [%s] \"GET /somepage HTTP/1.1\" 200 0 0.050/0.050 \"https://github.com/\" \"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko)\" - -\n",
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
		expected := fmt.Sprintf(tt.expected, time.Now().Format("02/Jan/2006:15:04:05 -0700"))
		*target = stringContainer{""}
		accessLogger.Handle(tt.handler)(blackHole{}, request)
		actual := target.string
		if actual != expected {
			t.Errorf("\nactual\n%s\nexpected\n%s", actual, expected)
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
