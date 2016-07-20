package httpaccesslog

import (
	"fmt"
	"log"
	"net/http"
	"testing"
	"time"
)

type ExpectationChecker struct {
	t        *testing.T
	Expected string
}

func (this ExpectationChecker) Write(p []byte) (n int, err error) {
	actual := string(p)
	expected := fmt.Sprintf(this.Expected, time.Now().Format("02/Jan/2006:15:04:05 -0700"))
	if actual != expected {
		this.t.Errorf("\nactual\n%s\nexpected\n%s", actual, expected)
	}

	return len(p), nil
}

type BlackHole struct {
}

func (this BlackHole) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (this BlackHole) WriteHeader(int) {
}

func (this BlackHole) Header() http.Header {
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

func TestServeMux(t *testing.T) {
	expectationChecker := &ExpectationChecker{t, ""}
	log.SetFlags(0)
	log.SetOutput(expectationChecker)
	http.HandleFunc("/usage", Handler(usageHandler))
	http.HandleFunc("/denied", Handler(deniedHandler))
	http.HandleFunc("/delayed", Handler(delayedHandler))
	go http.ListenAndServe(":5000", nil)
	expectationChecker.Expected = "127.0.0.1 - user [%s] \"GET /usage HTTP/1.1\" 200 78 0.000/0.000 \"-\" \"-\" - -\n"
	http.Get("http://user:pass@localhost:5000/usage")
	expectationChecker.Expected = "127.0.0.1 - - [%s] \"GET /denied HTTP/1.1\" 401 0 0.000/0.000 \"-\" \"-\" - -\n"
	http.Get("http://localhost:5000/denied")
	expectationChecker.Expected = "127.0.0.1 - - [%s] \"GET /delayed HTTP/1.1\" 200 0 0.050/0.050 \"-\" \"-\" - -\n"
	http.Get("http://localhost:5000/delayed")
}

func TestHandler(t *testing.T) {
	expectationChecker := &ExpectationChecker{t, ""}
	log.SetFlags(0)
	log.SetOutput(expectationChecker)
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
			http.MethodGet,
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
			http.MethodGet,
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
			http.MethodGet,
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
		expectationChecker.Expected = tt.expected
		Handler(tt.handler)(BlackHole{}, request)
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
			http.MethodGet,
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
			http.MethodGet,
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
			http.MethodGet,
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
