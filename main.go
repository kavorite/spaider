package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

    "regexp"
	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/gocolly/colly/v2"
)

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
    pathMatch     string
	startPath     string
	startURL      string
	allowedDomain string
	allowedExts   extensions
	defaultExts   = extensions{"", ".html", ".md", ".txt", ".rst"}
)

func main() {
    flag.StringVar(&pathMatch, "glob", "", "Pattern to match paths")
	flag.Parse()
    pathRegex = regexp.MustCompile(pathMatch);
	startURL = flag.Arg(0)
	fmt.Println(startURL)

	// Use default extensions if none provided
	if len(allowedExts) == 0 {
		allowedExts = defaultExts
	}

	parsedURL, err := url.Parse(startURL)
	if err != nil {
		log.Fatalf("Failed to parse URL: %v", err)
	}
	startPath = parsedURL.Path
	allowedDomain = parsedURL.Host

	c := colly.NewCollector(
		colly.AllowedDomains(allowedDomain),
		colly.UserAgent("github.com/kavorite/spaider"),
		colly.MaxBodySize(1<<19), // according to the HTTP Archive, 99% of text documents should be under this limit
		colly.Async(),
	)

	c.OnRequest(func(r *colly.Request) {
		fmt.Fprintln(os.Stderr, r.URL.String())
	})

	c.OnResponse(func(r *colly.Response) {
		path := r.Request.URL.Path
		mime := strings.ToLower(r.Headers.Get("Content-Type"))
		if !isAllowedExtension(path) || !isAllowedContentType(mime) || !isPathAllowed(path) {
			return
		}
		fmt.Printf("# %s:\n\n", r.Request.URL.String())
		text := string(r.Body)

		if strings.HasSuffix(r.Request.URL.Path, ".html") || strings.Contains(mime, "html") {
			text = toMarkdown(text)
		}

		fmt.Println(text)
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
