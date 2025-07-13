// module for scraping xbato.com
package xbato

import (
	"fmt"

	"github.com/gocolly/colly"
)

// get list of all the chapter URLss
func XbatoChapterUrls(mangaName string) ([]string, error) {
	var urls []string

	mangaURL := fmt.Sprintf("https://xbato.com/series/%s", mangaName)

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36"),
	)

	c.OnHTML("div.episode-list div.main a.visited.chapt", func(e *colly.HTMLElement) {
		href := e.Attr("href")
		if href != "" {
			fullURL := "https://xbato.com" + href
			urls = append(urls, fullURL)
		}
	})

	var scrapeErr error
	c.OnError(func(_ *colly.Response, err error) {
		scrapeErr = err
	})

	err := c.Visit(mangaURL)
	if err != nil {
		return nil, err
	}

	if scrapeErr != nil {
		return nil, scrapeErr
	}

	return urls, nil
}
