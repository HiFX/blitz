package blitzkrieg

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
	"strconv"
	"code.google.com/p/go.net/publicsuffix"
	"net/http/cookiejar"
)

const (
	VERSION = "1.0"
)

var (
	count          int    // Number of requests per client
	clients        int    // Number of clients to simulate
	duration       int    // Duration of the test
	rate           int    // Rate limit
	url            string // URL
	urlsFilePath   string // Input file containing Urls
	keepAlive      bool   // Http Keep-alive on/off flag
	gzip           bool   // Accept gzip compression
	needLogin      bool   // Login on/off flag - if enabled the first url from urlsFilePath is used for login
	connectTimeout int    // Connect timeout in milliseconds
	readTimeout    int    // Read timeout in milliseconds
	writeTimeout   int    // Write timeout in milliseconds
	showErr        bool   // Show errors
	outFormat      string // Output Format
	version        bool   // Display version
	help           bool   // Display help
)

type blitzRequest struct {
	url    string
	method string
	header http.Header
	body   string
}

func (req *blitzRequest) getHttpRequest() (hReq *http.Request) {
	hReq, _ = http.NewRequest(req.method, req.url, strings.NewReader(req.body))
	hReq.Header = req.header
	return
}

func init() {
	flag.IntVar(&count, "n", -1, "Number of requests")
	flag.IntVar(&count, "number", -1, "")
	flag.IntVar(&clients, "c", 100, "Number of clients to simulate")
	flag.IntVar(&clients, "clients", 100, "")
	flag.IntVar(&duration, "d", -1, "Duration of the test in seconds")
	flag.IntVar(&duration, "duration", -1, "")
	flag.IntVar(&rate, "r", 0, "Rate limit")
	flag.IntVar(&rate, "rate", 0, "")
	flag.StringVar(&url, "u", "", "URL to test")
	flag.StringVar(&url, "url", "", "")
	flag.StringVar(&urlsFilePath, "f", "", "URLs file")
	flag.StringVar(&urlsFilePath, "file", "", "")
	flag.BoolVar(&keepAlive, "k", true, "Do keep HTTP keep-alive on")
	flag.BoolVar(&keepAlive, "keep", true, "")
	flag.BoolVar(&gzip, "g", true, "Accept Gzip")
	flag.BoolVar(&gzip, "gzip", true, "")
	flag.BoolVar(&needLogin, "l", false, "Login on/off flag")
	flag.BoolVar(&needLogin, "login", false, "")
	flag.BoolVar(&showErr, "e", false, "")
	flag.BoolVar(&showErr, "err", false, "Display Errors")
	flag.IntVar(&connectTimeout, "tc", 5000, "Connect timeout in ms")
	flag.IntVar(&connectTimeout, "timeoutcon", 5000, "")
	flag.IntVar(&readTimeout, "tr", 5000, "Read timeout in ms")
	flag.IntVar(&readTimeout, "timeoutread", 5000, "")
	flag.IntVar(&writeTimeout, "tw", 5000, "Write timeout in ms")
	flag.IntVar(&writeTimeout, "timeoutwrite", 5000, "")
	flag.StringVar(&outFormat, "o", "", "")
	flag.StringVar(&outFormat, "output", "", "Output Format")
	flag.BoolVar(&version, "v", false, "Prints the version number")
	flag.BoolVar(&help, "h", false, "Show Help")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: blitz [options]\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "-c,  -clients        Clients           Number of clients to simulate[default 100].\n")
		fmt.Fprintf(os.Stderr, "-n,  -number         Number            Number of requests\n")
		fmt.Fprintf(os.Stderr, "-d,  -duration       Duration          Duration of the test in seconds.\n")
		fmt.Fprintf(os.Stderr, "-r,  -rate           Rate              Rate limit.\n")
		fmt.Fprintf(os.Stderr, "-u,  -url            URL               URL to test.\n")
		fmt.Fprintf(os.Stderr, "-f,  -file           URLs File         URLs file.\n")
		fmt.Fprintf(os.Stderr, "-k,  -keep           KeepAlive         HTTP keep-alive on/off [default true].\n")
		fmt.Fprintf(os.Stderr, "-g,  -gzip           GZip              Accept Gzip Compression [default true].\n")
		fmt.Fprintf(os.Stderr, "-l,  -login          Login             Do login on/off.\n")
		fmt.Fprintf(os.Stderr, "-tc, -timeoutcon     ConnectTimeout    Connect timeout in ms [default 5000].\n")
		fmt.Fprintf(os.Stderr, "-tr, -timeoutread    ReadTimeout       Read timeout in ms [default 5000].\n")
		fmt.Fprintf(os.Stderr, "-tw, -timeoutwrite   WriteTimeout      Write timeout in ms [default 5000].\n")
		fmt.Fprintf(os.Stderr, "-o,  -output         OutputFormat      [graph].\n")
		fmt.Fprintf(os.Stderr, "-e,  -err            ShowErr           Display Errors.\n")
		fmt.Fprintf(os.Stderr, "-v,  -version        Version           Prints the version number.\n")
		fmt.Fprintf(os.Stderr, "-h,  -help           Help              Prints this output.\n")
	}
}

// showVersion displays the version number
func showVersion() {
	fmt.Println("blitz", VERSION)
	fmt.Println("This is free software. There is NO warranty")
}

// NewBlitz returns a new Blitz after parsing/processing
// the command line parameters
func NewBlitz() (blitz *Blitz) {
	flag.Parse()
	if version {
		showVersion()
		os.Exit(0)
	}
	if help {
		flag.Usage()
		os.Exit(0)
	}
	if urlsFilePath == "" && url == "" {
		flag.Usage()
		os.Exit(0)
	}
	if count == -1 && duration == -1 {
		flag.Usage()
		os.Exit(0)
	}
	blitz = &Blitz{
		requests:       make([]*blitzRequest, 0),
		count:          math.MaxInt32,
		clients:        clients,
		rate:           rate,
		keepAlive:      keepAlive,
		gzip:           gzip,
		connectTimeout: connectTimeout,
		readTimeout:    readTimeout,
		writeTimeout:   writeTimeout,
	}
	if count != -1 {
		blitz.count = count
	}

	if urlsFilePath != "" {
		requests, err := readFile(urlsFilePath)
		if err != nil {
			log.Fatalf("Error reading file:%s Error: ", urlsFilePath, err)
			os.Exit(0)
		}
		if len(requests) == 0 {
			log.Fatalf("File has insufficient number of lines")
			os.Exit(0)
		}
		blitz.requests = requests
	}

	if url != "" {
		blitz.requests = append(blitz.requests, &blitzRequest{url: url, method: "GET"})
	}

	if duration != -1 {
		blitz.duration = duration
	}

	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
	return
}

// readFile reads a file containing urls and returns an array of
// http.Request elements
func readFile(path string) (requests []*blitzRequest, err error) {
	var (
		file         *os.File
		line         string
		arr          []string
		length       int
		req          *blitzRequest
		count        int
		loginCookies string
		tmpCookie    string
	)
	if file, err = os.Open(path); err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line = strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		arr = strings.Split(line, "\t")
		length = len(arr)
		req = &blitzRequest{url: arr[0], method: "GET", header: make(http.Header)}
		if length > 1 {
			req.method = arr[1]
		}
		switch req.method {
		case "POST":
			switch length {
			case 4:
				req.header = parseHeaders(arr[3])
				fallthrough
			case 3:
				req.header.Add("Content-Type", "application/x-www-form-urlencoded")
    			req.header.Add("Content-Length", strconv.Itoa(len(arr[2])))
				req.body = arr[2]
			}
		case "GET":
			if length > 2 {
				req.header = parseHeaders(arr[2])
			}
		}
		req.header.Set("User-Agent", "blitz "+VERSION)

		if loginCookies != "" {
			tmpCookie = req.header.Get("Cookie")
			if tmpCookie != "" {
				req.header.Set("Cookie", loginCookies+tmpCookie)
			} else {
				req.header.Set("Cookie", loginCookies)
			}
		}
		count++
		if needLogin && (count == 1) {
			var buffer bytes.Buffer
			lCookies, _ := doLogin(req)
			for _, lCookie := range lCookies {
				buffer.WriteString(lCookie.Name)
				buffer.WriteString("=")
				buffer.WriteString(lCookie.Value)
				buffer.WriteString("; ")
			}
			loginCookies = buffer.String()
		} else {
			requests = append(requests, req)
		}
	}
	err = scanner.Err()
	return
}

// parseHeaders parses the header string and returns an
// http.Header
func parseHeaders(headerStr string) (header http.Header) {
	var (
		hArr []string
	)
	header = make(http.Header)
	splitArr := strings.Split(strings.TrimLeft(headerStr, "-H"), "-H")
	for _, str := range splitArr {
		str = strings.Trim(str, " '")
		hArr = strings.SplitN(str, ":", 2)
		if len(hArr) > 1 {
			header.Set(strings.TrimSpace(hArr[0]), strings.TrimSpace(hArr[1]))
		} else {
			log.Fatalf("Error parsing header string: %s", headerStr)
			os.Exit(0)
		}
	}
	return
}

func doLogin(req *blitzRequest) ([]*http.Cookie, error) {
	options := cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	}
	jar, cerr := cookiejar.New(&options)
	if cerr != nil {
		log.Fatal(cerr)
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	tr.Dial = func(network string, address string) (conn net.Conn, err error) {
		return net.DialTimeout(network, address, time.Duration(connectTimeout)*time.Millisecond)
	}
	client := &http.Client{Transport: tr, Jar:jar}
	resp, err := client.Do(req.getHttpRequest())
	if resp != nil && err == nil {
		return resp.Cookies(), nil
	}
	return nil, err
}
