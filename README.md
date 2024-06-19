# What is this?

This program crawls a domain and tries to find all its documents for
conditioning an LLM. Currently, that means it takes anything with a `text/*`
content type that is under 500KiB, and if HTML, converts it to Markdown before
writing it to standard output. See [changelog.md](changelog.md) for added bells
and whistles.

# How do I use this?

> ⓘ **Note:** To ensure the following works as intended, install Go and make
> sure `$GOPATH/bin` is on your path.

```sh
go install github.com/kavorite/spaider
# you can use any documentation website
spaider https://flax.readthedocs.io/en/latest/ > summary.txt
# you're now free to upload the summary to an LLM inference platform somewhere
```

# Background

The program treats the starting URL as the "index." It may navigate through
hyperlinks on the same domain that aren't "children" of the relative URL given
in the starting path, but it will only actually print the content of those whose
paths are children of the path provided. This means its runtime depends on the
size of the website as navigable from the start page, but the size of its output
depends only on the size of the path hierarchy found under the chosen index.

## Acknowledgment
Big shoutout to [httparchive.org] for making all data from their crawls
publicly accessible under Google BigQuery, because otherwise, getting the 99th
percentile of the size of an HTML payload would be difficult. It wasn't 500KiB, 
it was 300 or so, but I rounded up to the nearest power of two to be safe.

[httparchive.org]: https://httparchive.org/faq#how-do-i-use-bigquery-to-write-custom-queries-over-the-data
