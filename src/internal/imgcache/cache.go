package imgcache

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Cache : Cache strcut
type Cache struct {
	path     string
	cacheURL string
	caching  bool
	images   map[string]string
	Queue    []string
	Cache    []string
	Image    imageFunc
	sync.RWMutex
}

type imageFunc struct {
	GetURL  func(string) string
	Caching func() error
	Remove  func() error
}

// New : New cache
func New(path, chacheURL string, caching bool) (c *Cache, err error) {

	c = &Cache{}

	c.images = make(map[string]string)
	c.path = path
	c.cacheURL = chacheURL
	c.caching = caching
	c.Queue = []string{}
	c.Cache = []string{}

	var queue []string

	c.Image.GetURL = func(src string) (cacheURL string) {

		c.Lock()
		defer c.Unlock()

		src = strings.Trim(src, "\r\n")

		if !c.caching {
			return src
		}

		u, err := url.Parse(src)
		if err != nil || len(filepath.Ext(u.Path)) == 0 {
			return src
		}

		var filename = fmt.Sprintf("%s%s", strToMD5(src), filepath.Ext(u.Path))
		if cacheURL, ok := c.images[fmt.Sprintf("%s%s", strToMD5(src), filepath.Ext(u.Path))]; ok {
			return cacheURL
		}

		if indexOfString(filename, c.Cache) == -1 {

			if indexOfString(src, c.Queue) == -1 {
				c.Queue = append(c.Queue, src)
			}

		} else {
			c.images[filename] = c.cacheURL + filename
			src = c.cacheURL + filename
		}

		/*
		   if _, err := os.Stat(c.path + filename); err != nil {
		     //c.images[filename] = c.cacheURL + filename
		     if indexOfString(src, c.Queue) == -1 {
		       c.Queue = append(c.Queue, src)
		     }
		   } else {
		     c.images[filename] = c.cacheURL + filename
		   }
		*/

		return src
	}

	c.Image.Caching = func() error {

		c.Lock()
		defer c.Unlock()

		var failed int
		var firstErr error

		for _, src := range c.Queue {

			var filename = fmt.Sprintf("%s%s%s%s", c.path, string(os.PathSeparator), strToMD5(src), filepath.Ext(src))

			if err := download(src, filename); err != nil {
				// The image stays in the queue and is retried on the next run
				failed++
				if firstErr == nil {
					firstErr = err
				}
				continue
			}

			u, err := url.Parse(src)
			if err == nil {
				c.images[fmt.Sprintf("%s%s", strToMD5(src), filepath.Ext(u.Path))] = c.cacheURL + filename
			}

			queue = append(queue, src)

		}

		var total = len(c.Queue)

		for _, q := range queue {
			c.Queue = removeStringFromSlice(q, c.Queue)
		}

		if failed > 0 {
			return fmt.Errorf(
				"image caching: %d of %d images could not be cached, first error: %w",
				failed,
				total,
				firstErr,
			)
		}

		return nil
	}

	c.Image.Remove = func() error {

		c.Lock()
		defer c.Unlock()

		files, err := os.ReadDir(c.path)
		if err != nil {
			return err
		}

		var errs []error

		for _, file := range files {

			if _, ok := c.images[file.Name()]; c.caching && ok {
				continue
			}

			if err := os.RemoveAll(c.path + file.Name()); err != nil {
				errs = append(errs, err)
			}

		}

		return errors.Join(errs...)
	}

	files, err := os.ReadDir(c.path)
	if err != nil {
		return
	}

	for _, file := range files {
		c.Cache = append(c.Cache, file.Name())
	}

	return
}

// download saves the image at src to filename. A partially written file is removed.
func download(src, filename string) (err error) {

	resp, err := http.Get(src)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", src, resp.Status)
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}

	_, err = io.Copy(file, resp.Body)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		// Do not leave a broken image in the cache
		return errors.Join(err, os.Remove(filename))
	}

	return nil
}
