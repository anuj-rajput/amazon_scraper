package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/anaskhan96/soup"
	"github.com/joho/godotenv"
)

// Product represents Amazon product information
type Product struct {
	Title       string    `json:"title"`
	Price       string    `json:"price"`
	Rating      float64   `json:"rating"`
	Description string    `json:"description"`
	Reviews     []Review  `json:"reviews,omitempty"`
}

// Review represents a product review
type Review struct {
	Author   string  `json:"author"`
	Date     string  `json:"date"`
	Rating   float64 `json:"rating"`
	Title    string  `json:"title"`
	Content  string  `json:"content"`
	Verified bool    `json:"verified"`
}

// Options for command-line flags
type Options struct {
	Details bool
	Reviews bool
	Count   int
	Sort    string
	Region  string
}

// Get product ID and domain from Amazon URL
func getProductIDAndDomain(url string) (string, string) {
	// Match ASIN patterns in Amazon URLs from any region
	patterns := []string{
		`amazon\.[a-z.]+/([A-Za-z0-9-]+/)?dp/([A-Z0-9]{10})`,
		`amazon\.[a-z.]+/gp/product/([A-Z0-9]{10})`,
		`amazon\.[a-z.]+/([A-Za-z0-9-]+/)?product/([A-Z0-9]{10})`,
		`amzn\.[a-z]+/([A-Z0-9]{10})`, // Short URLs
	}

	domainPattern := regexp.MustCompile(`https?://(?:www\.)?([a-zA-Z0-9.-]+)`)
	domainMatch := domainPattern.FindStringSubmatch(url)
	domain := "amazon.com" // Default domain
	
	if len(domainMatch) > 1 {
		domain = domainMatch[1]
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		match := re.FindStringSubmatch(url)
		if len(match) > 0 {
			// Return the last capture group which contains the ASIN
			return match[len(match)-1], domain
		}
	}
	return "", domain
}

// Create HTTP client with custom headers to avoid detection
func createHTTPClient() *http.Client {
	return &http.Client{
		Timeout: time.Second * 30,
	}
}

// Create request with custom headers
func createRequest(url string) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Extract domain from URL to set appropriate Accept-Language
	domainPattern := regexp.MustCompile(`https?://(?:www\.)?([a-zA-Z0-9.-]+)`)
	domainMatch := domainPattern.FindStringSubmatch(url)
	
	// Default language is English
	acceptLanguage := "en-US,en;q=0.5"
	
	// Set appropriate language based on domain
	if len(domainMatch) > 1 {
		domain := domainMatch[1]
		switch {
		case strings.Contains(domain, "amazon.de"):
			acceptLanguage = "de-DE,de;q=0.9,en;q=0.8"
		case strings.Contains(domain, "amazon.fr"):
			acceptLanguage = "fr-FR,fr;q=0.9,en;q=0.8"
		case strings.Contains(domain, "amazon.it"):
			acceptLanguage = "it-IT,it;q=0.9,en;q=0.8"
		case strings.Contains(domain, "amazon.es"):
			acceptLanguage = "es-ES,es;q=0.9,en;q=0.8"
		case strings.Contains(domain, "amazon.co.jp"):
			acceptLanguage = "ja-JP,ja;q=0.9,en;q=0.8"
		case strings.Contains(domain, "amazon.co.uk"):
			acceptLanguage = "en-GB,en;q=0.9"
		case strings.Contains(domain, "amazon.ca"):
			acceptLanguage = "en-CA,en;q=0.9,fr-CA;q=0.8"
		case strings.Contains(domain, "amazon.com.br"):
			acceptLanguage = "pt-BR,pt;q=0.9,en;q=0.8"
		case strings.Contains(domain, "amazon.com.mx"):
			acceptLanguage = "es-MX,es;q=0.9,en;q=0.8"
		case strings.Contains(domain, "amazon.nl"):
			acceptLanguage = "nl-NL,nl;q=0.9,en;q=0.8"
		case strings.Contains(domain, "amazon.se"):
			acceptLanguage = "sv-SE,sv;q=0.9,en;q=0.8"
		case strings.Contains(domain, "amazon.com.au"):
			acceptLanguage = "en-AU,en;q=0.9"
		case strings.Contains(domain, "amazon.in"):
			acceptLanguage = "en-IN,en;q=0.9,hi;q=0.8"
		}
	}

	// Add headers to mimic a real browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", acceptLanguage)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Cache-Control", "max-age=0")

	return req, nil
}

// Fetch the HTML content of a page
func fetchHTML(url string) (string, error) {
	client := createHTTPClient()
	req, err := createRequest(url)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// Get product details from product page
func getProductDetails(productID string, domain string) (Product, error) {
	product := Product{}
	url := fmt.Sprintf("https://www.%s/dp/%s", domain, productID)
	
	html, err := fetchHTML(url)
	if err != nil {
		return product, err
	}

	doc := soup.HTMLParse(html)

	// Extract product title (multiple possible selectors)
	titleSelectors := []string{
		"span#productTitle",
		"h1#title",
		"h1.a-spacing-none",
	}
	for _, selector := range titleSelectors {
		titleElem := doc.Find(selector)
		if titleElem.Error == nil {
			title := strings.TrimSpace(titleElem.Text())
			if title != "" {
				product.Title = title
				break
			}
		}
	}
	
	// Extract product price (try multiple selectors as Amazon's structure changes)
	priceSelectors := [][]string{
		// Selector type, selector
		{"class", "a-price"},
		{"class", "a-price a-text-price"},
		{"id", "priceblock_ourprice"},
		{"id", "priceblock_dealprice"},
		{"id", "price"},
		{"class", "a-color-price"},
	}

	for _, selectorPair := range priceSelectors {
		selectorType, selector := selectorPair[0], selectorPair[1]
		var priceElem soup.Root
		
		if selectorType == "id" {
			priceElem = doc.Find("span", "id", selector)
		} else {
			priceElem = doc.Find("span", "class", selector)
		}
		
		if priceElem.Error == nil {
			// Try to get the price from the found element
			priceText := strings.TrimSpace(priceElem.Text())
			if priceText != "" {
				product.Price = priceText
				break
			}
			
			// If no text directly, try to find the offscreen price
			offscreenPrice := priceElem.Find("span", "class", "a-offscreen")
			if offscreenPrice.Error == nil {
				priceText = strings.TrimSpace(offscreenPrice.Text())
				if priceText != "" {
					product.Price = priceText
					break
				}
			}
		}
	}
	
	// If price is still empty, try a more general approach
	if product.Price == "" {
		allPriceSpans := doc.FindAll("span", "class", "a-offscreen")
		for _, span := range allPriceSpans {
			text := strings.TrimSpace(span.Text())
			// Make sure it starts with a currency symbol
			if strings.ContainsAny(text[:1], "$£€¥") {
				product.Price = text
				break
			}
		}
	}

	// Extract product rating (try multiple selectors)
	ratingSelectors := [][]string{
		{"id", "acrPopover"},
		{"class", "a-icon-star"},
		{"class", "a-star-medium-4"},
	}
	
	for _, selectorPair := range ratingSelectors {
		selectorType, selector := selectorPair[0], selectorPair[1]
		var ratingElem soup.Root
		
		if selectorType == "id" {
			ratingElem = doc.Find("span", "id", selector)
			if ratingElem.Error == nil {
				// Try to extract from title attribute
				ratingStr, exists := ratingElem.Attrs()["title"]
				if exists && strings.Contains(ratingStr, "out of 5 stars") {
					parts := strings.Split(ratingStr, " ")
					if len(parts) > 0 {
						product.Rating, _ = strconv.ParseFloat(parts[0], 64)
						break
					}
				}
			}
		} else {
			ratingElems := doc.FindAll("i", "class", selector)
			if len(ratingElems) > 0 {
				ratingText := ratingElems[0].Text()
				if strings.Contains(ratingText, "out of 5 stars") {
					parts := strings.Split(ratingText, " ")
					if len(parts) > 0 {
						product.Rating, _ = strconv.ParseFloat(parts[0], 64)
						break
					}
				}
				
				// Try another method - from the class name
				for _, elem := range ratingElems {
					classes, exists := elem.Attrs()["class"]
					if exists && strings.Contains(classes, "a-star-") {
						re := regexp.MustCompile(`a-star-(\d)(?:[-.](\d))?`)
						matches := re.FindStringSubmatch(classes)
						if len(matches) >= 2 {
							major, _ := strconv.ParseFloat(matches[1], 64)
							minor := 0.0
							if len(matches) >= 3 && matches[2] != "" {
								minor, _ = strconv.ParseFloat("0."+matches[2], 64)
							}
							product.Rating = major + minor
							break
						}
					}
				}
			}
		}
	}

	// Extract product description (try multiple locations)
	descriptionSelectors := []string{
		"div#productDescription",
		"div#dpx-product-description_feature_div",
		"div#feature-bullets",
		"div#dpx-feature-bullets_feature_div",
		"div#bookDescription_feature_div",
		"div#aplus",
	}
	
	for _, selector := range descriptionSelectors {
		descElem := doc.Find(selector)
		if descElem.Error == nil {
			desc := strings.TrimSpace(descElem.Text())
			if desc != "" {
				// Clean up the description - remove excess whitespace
				desc = regexp.MustCompile(`\s+`).ReplaceAllString(desc, " ")
				product.Description = desc
				break
			}
		}
	}
	
	// If we still don't have a description, look for bullet points
	if product.Description == "" {
		bulletPoints := doc.FindAll("li", "class", "a-spacing-mini")
		var bulletTexts []string
		
		for _, bullet := range bulletPoints {
			bulletText := strings.TrimSpace(bullet.Text())
			if bulletText != "" {
				bulletTexts = append(bulletTexts, bulletText)
			}
		}
		
		if len(bulletTexts) > 0 {
			product.Description = strings.Join(bulletTexts, " • ")
		}
	}

	return product, nil
}

// Get product reviews
func getProductReviews(productID string, domain string, count int, sort string) ([]Review, error) {
	reviews := []Review{}
	
	// Map sort parameter to Amazon's sort values
	sortParam := "helpful"
	switch strings.ToLower(sort) {
		case "recent":
			sortParam = "recent"
		case "rating":
			sortParam = "rating"
	}
	
	// Determine how many pages to fetch based on count (10 reviews per page)
	pages := (count + 9) / 10
	if pages > 10 {  // Limit to 10 pages
		pages = 10
	}
	
	for page := 1; page <= pages; page++ {
		if len(reviews) >= count {
			break
		}
		
		url := fmt.Sprintf("https://www.%s/product-reviews/%s/?pageNumber=%d&sortBy=%s", 
			domain, productID, page, sortParam)
		
		html, err := fetchHTML(url)
		if err != nil {
			return reviews, err
		}
		
		doc := soup.HTMLParse(html)
		reviewElems := doc.FindAll("div", "data-hook", "review")
		
		for _, reviewElem := range reviewElems {
			if len(reviews) >= count {
				break
			}
			
			review := Review{}
			
			// Extract review author
			authorElem := reviewElem.Find("span", "class", "a-profile-name")
			if authorElem.Error == nil {
				review.Author = strings.TrimSpace(authorElem.Text())
			}
			
			// Extract review date
			dateElem := reviewElem.Find("span", "data-hook", "review-date")
			if dateElem.Error == nil {
				review.Date = strings.TrimSpace(dateElem.Text())
			}
			
			// Extract review rating
			ratingElem := reviewElem.Find("i", "data-hook", "review-star-rating")
			if ratingElem.Error == nil {
				ratingStr := ratingElem.Text()
				if strings.Contains(ratingStr, "out of 5 stars") {
					ratingVal := strings.Split(ratingStr, " ")[0]
					review.Rating, _ = strconv.ParseFloat(ratingVal, 64)
				}
			}
			
			// Extract review title
			titleElem := reviewElem.Find("a", "data-hook", "review-title")
			if titleElem.Error == nil {
				review.Title = strings.TrimSpace(titleElem.Text())
			}
			
			// Extract review content
			contentElem := reviewElem.Find("span", "data-hook", "review-body")
			if contentElem.Error == nil {
				review.Content = strings.TrimSpace(contentElem.Text())
			}
			
			// Check if verified purchase
			verifiedElem := reviewElem.Find("span", "data-hook", "avp-badge")
			review.Verified = verifiedElem.Error == nil
			
			reviews = append(reviews, review)
		}
		
		// Add delay to prevent rate limiting
		time.Sleep(time.Second * 2)
	}
	
	return reviews, nil
}

func main() {
	options := &Options{}
	flag.BoolVar(&options.Details, "details", false, "Output only the product details")
	flag.BoolVar(&options.Reviews, "reviews", false, "Output only the product reviews")
	flag.IntVar(&options.Count, "count", 10, "Number of reviews to fetch (default: 10)")
	flag.StringVar(&options.Sort, "sort", "helpful", "Sort reviews by: helpful, recent, or rating (default: helpful)")
	flag.StringVar(&options.Region, "region", "", "Override region/domain (e.g., amazon.de, amazon.co.uk)")
	flag.Parse()

	if flag.NArg() == 0 {
		log.Fatal("Error: No Amazon URL provided.")
	}

	url := flag.Arg(0)
	productID, domain := getProductIDAndDomain(url)
	
	// Override domain if region flag is provided
	if options.Region != "" {
		if !strings.Contains(options.Region, "amazon.") {
			domain = "amazon." + options.Region
		} else {
			domain = options.Region
		}
	}
	
	if productID == "" {
		log.Fatal("Error: Invalid Amazon URL or couldn't extract product ID.")
	}
	
	log.Printf("Using Amazon domain: %s", domain)
	log.Printf("Product ID (ASIN): %s", productID)

	// Try to load API key if available (for future API integration)
	home_dir, err := os.UserHomeDir()
	if err == nil {
		env_file := home_dir + "/.config/fabric/.env"
		_ = godotenv.Load(env_file)
	}

	product, err := getProductDetails(productID, domain)
	if err != nil {
		log.Printf("Warning: Error fetching product details: %v", err)
	}

	var reviews []Review
	if options.Reviews || (!options.Details && !options.Reviews) {
		var err error
		reviews, err = getProductReviews(productID, domain, options.Count, options.Sort)
		if err != nil {
			log.Printf("Warning: Error fetching reviews: %v", err)
		}
		product.Reviews = reviews
	}

	// Output based on flags
	if options.Details {
		// Remove reviews to show only details
		product.Reviews = nil
		jsonOutput, _ := json.MarshalIndent(product, "", "  ")
		fmt.Println(string(jsonOutput))
	} else if options.Reviews {
		jsonReviews, _ := json.MarshalIndent(reviews, "", "  ")
		fmt.Println(string(jsonReviews))
	} else {
		jsonOutput, _ := json.MarshalIndent(product, "", "  ")
		fmt.Println(string(jsonOutput))
	}
}