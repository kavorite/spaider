package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"

	"regexp"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/gocolly/colly/v2"
)

type docset struct {
	set map[[16]byte]struct{}
	sync.RWMutex
}

type response struct {
	name string
	body string
}

func (rsp *response) StreamTo(dst io.Writer) {
	fmt.Fprintf(dst, "# %s:\n\n%s\n\n", rsp.name, rsp.body)
}

func (s *docset) Add(paragraph string) {
	s.Lock()
	s.set[md5.Sum([]byte(paragraph))] = struct{}{}
	s.Unlock()
}

func (s *docset) Has(paragraph string) bool {
	s.RLock()
	_, ok := s.set[md5.Sum([]byte(paragraph))]
	s.RUnlock()
	return ok
}

func (s *docset) Dedup(document string) string {
	buffer := strings.Builder{}
	for i, p := range strings.Split(document, "\n\n") {
		p = strings.TrimSpace(p)
		if !s.Has(p) {
			s.Add(p)
			if i != 0 {
				buffer.WriteString("\n\n")
			}
			buffer.WriteString(p)
		}
	}
	return strings.TrimSpace(buffer.String())
}

// Extensions to filter
type extensions []string

func (e *extensions) String() string {
	return fmt.Sprint(*e)
}

func (e *extensions) Set(value string) error {
	*e = append(*e, value)
	return nil
}

var (
	converter     = md.NewConverter("", true, nil)
	pathRegex     *regexp.Regexp
	paragraph     docset = docset{set: make(map[[16]byte]struct{}, 8192)}
	pathMatch     string
	startURL      string
	allowedDomain string
	allowedExts   extensions
	defaultExts   = extensions{"", ".html", ".md", ".txt", ".rst"}
	synchronous   bool
)

func main() {
	flag.Var(&allowedExts, "exts", fmt.Sprintf("Allowed file extensions. Defaults to %s.", defaultExts))
	flag.StringVar(&pathMatch, "glob", "", "Pattern to match paths")
	flag.BoolVar(&synchronous, "sync", false, "Make output deterministic by crawling pages synchronously")
	flag.Parse()
	if len(allowedExts) == 0 {
		allowedExts = defaultExts
	}
	pathRegex = regexp.MustCompile(pathMatch)
	startURL = flag.Arg(0)

	// Use default extensions if none provided
	if len(allowedExts) == 0 {
		allowedExts = defaultExts
	}

	parsedURL, err := url.Parse(startURL)
	if err != nil {
		log.Fatalf("Failed to parse URL: %v", err)
	}
	allowedDomain = parsedURL.Host

	options := []colly.CollectorOption{
		colly.AllowedDomains(allowedDomain),
		colly.UserAgent("github.com/kavorite/spaider"),
		colly.MaxBodySize(1 << 19), // according to the HTTP Archive, 99% of text documents should be under this limit
	}
	if !synchronous {
		options = append(options, colly.Async())
	}
	c := colly.NewCollector(options...)

	c.OnRequest(func(r *colly.Request) {
		fmt.Fprintln(os.Stderr, r.URL.String())
	})

	c.OnResponse(func(r *colly.Response) {
		path := r.Request.URL.Path
		mime := strings.ToLower(r.Headers.Get("Content-Type"))
		if !isAllowedExtension(path) || !isAllowedContentType(mime) || !isPathAllowed(path) {
			return
		}
		text := string(r.Body)

		if strings.HasSuffix(r.Request.URL.Path, ".html") || strings.Contains(mime, "html") {
			text = toMarkdown(text)
		}
		text = paragraph.Dedup(text)
		if text != "" {
			resp := response{name: r.Request.URL.String(), body: text}
			resp.StreamTo(os.Stdout)
		}
	})

	c.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Attr("href")
		if _, err := url.Parse(link); err == nil {
			e.Request.Visit(e.Request.AbsoluteURL(link))
		}
	})

	c.Visit(startURL)
	c.Wait()
}

func isPathAllowed(path string) bool {
	if pathRegex != nil {
		return pathRegex.MatchString(path)
	} else {
		return true
	}
}

func isAllowedExtension(path string) bool {
	for _, ext := range allowedExts {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func isAllowedContentType(mime string) bool {
	return strings.HasPrefix(mime, "text/")
}

func toMarkdown(html string) string {
	mkdn, _ := converter.ConvertString(html)
	return mkdn
}
