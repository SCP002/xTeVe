// Package imgcache caches the logos and posters of the channels in a local folder, from where xTeVe serves them.
package imgcache

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// downloads is the number of images that are downloaded at the same time.
const downloads = 8

// Cache knows the cached images of one build. GetURL queues the images that are not cached yet, Download
// downloads them. GetURL does not wait for running downloads.
type Cache struct {
	dir     string
	url     string
	caching bool
	client  *http.Client

	mu sync.Mutex
	// files are the names of the images in the folder
	files map[string]bool
	// used are the names of the cached images the build refers to
	used map[string]bool
	// queue are the sources of the images to download, in the order they were asked for
	queue  []string
	queued map[string]bool
}

// New returns the cache of the images in dir, served at cacheURL. Without caching, GetURL returns the sources and
// Remove empties the folder.
func New(dir, cacheURL string, caching bool) (*Cache, error) {
	var c = &Cache{
		dir:     dir,
		url:     cacheURL,
		caching: caching,
		client:  &http.Client{Timeout: 30 * time.Second},
		files:   make(map[string]bool),
		used:    make(map[string]bool),
		queued:  make(map[string]bool),
	}

	var entries, err = os.ReadDir(dir)
	if err != nil {
		return c, err
	}
	for _, e := range entries {
		c.files[e.Name()] = true
	}
	return c, nil
}

// name returns the file name of an image: the MD5 of its source with the extension of the URL path, "" if the
// source has no extension.
func name(src string) string {
	var u, err = url.Parse(src)
	if err != nil || filepath.Ext(u.Path) == "" {
		return ""
	}
	var sum = md5.Sum([]byte(src))
	return hex.EncodeToString(sum[:]) + filepath.Ext(u.Path)
}

// GetURL returns the URL of the cached image, or src if it is not cached (yet). An image that is not cached is
// queued for Download.
func (c *Cache) GetURL(src string) string {
	src = strings.Trim(src, "\r\n")
	if !c.caching {
		return src
	}
	var n = name(src)
	if n == "" {
		return src
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.files[n] {
		c.used[n] = true
		return c.url + n
	}
	if !c.queued[src] {
		c.queued[src] = true
		c.queue = append(c.queue, src)
	}
	return src
}

// Queued returns the number of images waiting for Download.
func (c *Cache) Queued() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.queue)
}

// Download downloads the queued images and returns how many it downloaded. An image whose file is in the folder
// already, downloaded for another build meanwhile, is not downloaded again. Images that fail stay in the queue and
// are tried again on the next call.
func (c *Cache) Download() (int, error) {
	c.mu.Lock()
	var queue = c.queue
	c.mu.Unlock()

	var indexes = make(chan int)
	var errs = make([]error, len(queue))
	var downloaded atomic.Int64
	var wg sync.WaitGroup
	for range min(downloads, len(queue)) {
		wg.Go(func() {
			for i := range indexes {
				var src, n = queue[i], name(queue[i])
				var file = filepath.Join(c.dir, n)
				if _, err := os.Stat(file); err != nil {
					if err := c.download(src, file); err != nil {
						errs[i] = err
						continue
					}
					downloaded.Add(1)
				}
				c.mu.Lock()
				c.files[n] = true
				c.used[n] = true
				delete(c.queued, src)
				c.mu.Unlock()
			}
		})
	}
	for i := range queue {
		indexes <- i
	}
	close(indexes)
	wg.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()
	var left = c.queue[:0:0]
	for _, src := range c.queue {
		if c.queued[src] {
			left = append(left, src)
		}
	}
	c.queue = left

	var failed []error
	for _, err := range errs {
		if err != nil {
			failed = append(failed, err)
		}
	}
	if len(failed) > 0 {
		return int(downloaded.Load()), fmt.Errorf(
			"image caching: %d of %d images could not be cached, first error: %w", len(failed), len(queue), failed[0])
	}
	return int(downloaded.Load()), nil
}

// Remove deletes the images in the folder that the build does not refer to, or all images without caching.
func (c *Cache) Remove() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var entries, err = os.ReadDir(c.dir)
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		if c.caching && c.used[e.Name()] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(c.dir, e.Name())); err != nil {
			errs = append(errs, err)
		}
		delete(c.files, e.Name())
	}
	return errors.Join(errs...)
}

// download saves the image at src to filename. A partially written file is removed.
func (c *Cache) download(src, filename string) (err error) {
	resp, err := c.client.Get(src)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", src, resp.Status)
	}

	// Written to a temporary file first, so that a running build never serves a half written image
	file, err := os.CreateTemp(filepath.Dir(filename), filepath.Base(filename)+".*.tmp")
	if err != nil {
		return err
	}
	_, err = io.Copy(file, resp.Body)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(file.Name(), filename)
	}
	if err != nil {
		return errors.Join(err, os.Remove(file.Name()))
	}
	return nil
}
