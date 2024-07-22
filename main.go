package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"io"
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

type rgxlist []*regexp.Regexp

func (r *rgxlist) Set(value string) error {
	v, err := regexp.Compile(value)
	if err != nil {
		return err
	}
	*r = append(*r, v)
	return nil
}

func (s *rgxlist) String() string {
	return fmt.Sprint(*s)
}

type strlist []string

func (s *strlist) String() string {
	return fmt.Sprint(*s)
}

func (s *strlist) Set(value string) error {
	*s = append(*s, value)
	return nil
}

var (
	converter          = md.NewConverter("", true, nil)
	paragraph   docset = docset{set: make(map[[16]byte]struct{}, 8192)}
	verbose     bool
	allowedGlob rgxlist
	removedGlob rgxlist
	startURL    string
	defaultExts = strlist{"", ".html", ".md", ".txt", ".rst"}
	synchronous bool
	allowedExts strlist
	maxd        int
)

func init() {
	flag.Var(&allowedGlob, "glob", "Patterns that match URLs to visit.")
	flag.Var(&removedGlob, "filt", "Patterns that filter URLs to visit.")
	flag.Var(&allowedExts, "exts", "File extensions that permit results to be printed.")
	flag.IntVar(&maxd, "maxd", -1, "Maximum depth.")
	flag.BoolVar(&verbose, "verbose", false, "Print visited URLs")
	flag.BoolVar(&synchronous, "sync", false, "Make output deterministic and limit server load by crawling pages synchronously")
}

func main() {
	flag.Parse()
	startURL = flag.Arg(0)
	if len(allowedExts) == 0 {
		allowedExts = defaultExts
	}

	parsedURL, err := url.Parse(startURL)
	if err != nil {
		panic(fmt.Errorf("fatal: parse start url: %v", err))
	}

	if len(allowedGlob) == 0 {
		allowedGlob = rgxlist{regexp.MustCompile(parsedURL.JoinPath(".*").String())}
	}
	// Use default extensions if none provided
	if len(allowedExts) == 0 {
		allowedExts = defaultExts
	}

	options := []colly.CollectorOption{
		colly.URLFilters(allowedGlob...),
		colly.DisallowedURLFilters(removedGlob...),
		colly.UserAgent("github.com/kavorite/spaider"),
	}
	if maxd > 0 {
		options = append(options, colly.MaxDepth(maxd))
	}
	if !synchronous {
		options = append(options, colly.Async())
	}
	c := colly.NewCollector(options...)

	if verbose {
		c.OnRequest(func(r *colly.Request) {
			fmt.Fprintln(os.Stderr, r.URL.String())
		})
	}

	c.OnResponse(func(r *colly.Response) {
		path := r.Request.URL.Path
		mime := strings.ToLower(r.Headers.Get("Content-Type"))
		if !isAllowedExtension(path) || !isAllowedContentType(mime) {
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
		absoluteURL := e.Request.AbsoluteURL(link)
		if absoluteURL == "" {
			return
		}
		if _, err := url.Parse(absoluteURL); err == nil {
			for _, pattern := range removedGlob {
				if pattern.MatchString(absoluteURL) {
					return
				}
			}
			for _, pattern := range allowedGlob {
				if !pattern.MatchString(absoluteURL) {
					return
				}
			}
			e.Request.Visit(absoluteURL)
		}
	})

	c.Visit(startURL)
	c.Wait()
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
