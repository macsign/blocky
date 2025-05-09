package lists

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"

	"github.com/0xERR0R/blocky/config"
	. "github.com/0xERR0R/blocky/evt"
	. "github.com/0xERR0R/blocky/helpertest"
	"github.com/0xERR0R/blocky/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus/hooks/test"
)

var _ = Describe("Downloader", func() {
	var (
		sutConfig                     config.Downloader
		sut                           *httpDownloader
		failedDownloadCountEvtChannel chan string
		loggerHook                    *test.Hook
	)
	BeforeEach(func() {
		var err error

		sutConfig, err = config.WithDefaults[config.Downloader]()
		Expect(err).Should(Succeed())

		failedDownloadCountEvtChannel = make(chan string, 5)
		// collect received events in the channel
		fn := func(url string) {
			failedDownloadCountEvtChannel <- url
		}
		Expect(Bus().Subscribe(CachingFailedDownloadChanged, fn)).Should(Succeed())
		DeferCleanup(func() {
			Expect(Bus().Unsubscribe(CachingFailedDownloadChanged, fn)).Should(Succeed())
		})

		loggerHook = test.NewGlobal()
		log.Log().AddHook(loggerHook)
		DeferCleanup(loggerHook.Reset)
	})

	JustBeforeEach(func() {
		sut = newDownloader(sutConfig, nil)
	})

	Describe("NewDownloader", func() {
		It("Should use provided parameters", func() {
			transport := new(http.Transport)

			sut = NewDownloader(
				config.Downloader{
					Attempts: 5,
					Cooldown: config.Duration(2 * time.Second),
					Timeout:  config.Duration(5 * time.Second),
				},
				transport,
			).(*httpDownloader)

			Expect(sut.cfg.Attempts).Should(BeNumerically("==", 5))
			Expect(sut.cfg.Timeout).Should(BeNumerically("==", 5*time.Second))
			Expect(sut.cfg.Cooldown).Should(BeNumerically("==", 2*time.Second))
			Expect(sut.client.Transport).Should(BeIdenticalTo(transport))
		})
	})

	Describe("Download of a file", func() {
		var server *httptest.Server
		When("Download was successful", func() {
			BeforeEach(func() {
				server = TestServer("line.one\nline.two")
				sut = newDownloader(sutConfig, nil)
			})
			It("Should return all lines from the file", func(ctx context.Context) {
				reader, err := sut.DownloadFile(ctx, server.URL)

				Expect(err).Should(Succeed())
				Expect(reader).ShouldNot(BeNil())
				DeferCleanup(reader.Close)
				buf := new(strings.Builder)
				_, err = io.Copy(buf, reader)
				Expect(err).Should(Succeed())
				Expect(buf.String()).Should(Equal("line.one\nline.two"))
			})
		})
		When("Server returns NOT_FOUND (404)", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
					rw.WriteHeader(http.StatusNotFound)
				}))
				DeferCleanup(server.Close)

				sutConfig.Attempts = 3
			})
			It("Should return error", func(ctx context.Context) {
				reader, err := sut.DownloadFile(ctx, server.URL)

				Expect(err).Should(HaveOccurred())
				Expect(reader).Should(BeNil())
				Expect(err.Error()).Should(Equal("got status code 404"))
				Expect(failedDownloadCountEvtChannel).Should(HaveLen(3))
				Expect(failedDownloadCountEvtChannel).Should(Receive(Equal(server.URL)))
			})
		})
		When("Wrong URL is defined", func() {
			BeforeEach(func() {
				sutConfig.Attempts = 1
			})
			It("Should return error", func(ctx context.Context) {
				_, err := sut.DownloadFile(ctx, "somewrongurl")

				Expect(err).Should(HaveOccurred())
				Expect(loggerHook.LastEntry().Message).Should(ContainSubstring("Can't download file: "))
				// failed download event was emitted only once
				Expect(failedDownloadCountEvtChannel).Should(HaveLen(1))
				Expect(failedDownloadCountEvtChannel).Should(Receive(Equal("somewrongurl")))
			})
		})

		When("If timeout occurs on first request", func() {
			var attempt uint64 = 1

			BeforeEach(func() {
				sutConfig = config.Downloader{
					Timeout:  config.Duration(20 * time.Millisecond),
					Attempts: 3,
					Cooldown: config.Duration(time.Millisecond),
				}

				// should produce a timeout on first attempt
				server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
					a := atomic.LoadUint64(&attempt)
					atomic.AddUint64(&attempt, 1)
					if a == 1 {
						time.Sleep(500 * time.Millisecond)
					} else {
						_, err := rw.Write([]byte("blocked1.com"))
						Expect(err).Should(Succeed())
					}
				}))
			})
			It("Should perform a retry and return file content", func(ctx context.Context) {
				reader, err := sut.DownloadFile(ctx, server.URL)
				Expect(err).Should(Succeed())
				Expect(reader).ShouldNot(BeNil())
				DeferCleanup(reader.Close)

				buf := new(strings.Builder)
				_, err = io.Copy(buf, reader)
				Expect(err).Should(Succeed())
				Expect(buf.String()).Should(Equal("blocked1.com"))

				// failed download event was emitted only once
				Expect(failedDownloadCountEvtChannel).Should(HaveLen(1))
				Expect(failedDownloadCountEvtChannel).Should(Receive(Equal(server.URL)))
				Expect(loggerHook.LastEntry().Message).Should(ContainSubstring("Temporary network err / Timeout occurred: "))
			})
		})
		When("If timeout occurs on all request", func() {
			BeforeEach(func() {
				sutConfig = config.Downloader{
					Timeout:  config.Duration(10 * time.Millisecond),
					Attempts: 3,
					Cooldown: config.Duration(time.Millisecond),
				}

				// should always produce a timeout
				server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
					time.Sleep(20 * time.Millisecond)
				}))
			})
			It("Should perform a retry until max retry attempt count is reached and return TransientError",
				func(ctx context.Context) {
					reader, err := sut.DownloadFile(ctx, server.URL)
					Expect(err).Should(HaveOccurred())
					Expect(errors.As(err, new(*TransientError))).Should(BeTrue())
					Expect(err.Error()).Should(ContainSubstring("Timeout"))
					Expect(reader).Should(BeNil())

					// failed download event was emitted 3 times
					Expect(failedDownloadCountEvtChannel).Should(HaveLen(3))
					Expect(failedDownloadCountEvtChannel).Should(Receive(Equal(server.URL)))
				})
		})
		When("DNS resolution of passed URL fails", func() {
			BeforeEach(func() {
				sutConfig = config.Downloader{
					Timeout:  config.Duration(500 * time.Millisecond),
					Attempts: 3,
					Cooldown: 200 * config.Duration(time.Millisecond),
				}
			})
			It("Should perform a retry until max retry attempt count is reached and return DNS-related error",
				func(ctx context.Context) {
					reader, err := sut.DownloadFile(ctx, "http://xyz.example.com")
					Expect(err).Should(HaveOccurred())

					// Check if it's a DNS error or contains DNS-related message
					var dnsError *net.DNSError
					isDNSError := errors.As(err, &dnsError)
					containsDNSErrorMessage := strings.Contains(strings.ToLower(err.Error()), "lookup") ||
						strings.Contains(strings.ToLower(err.Error()), "dns") ||
						strings.Contains(strings.ToLower(err.Error()), "resolve") ||
						strings.Contains(strings.ToLower(err.Error()), "unknown host")

					Expect(isDNSError || containsDNSErrorMessage).Should(BeTrue(),
						"expected DNS-related error, got: %v", err)
					Expect(reader).Should(BeNil())

					// failed download event was emitted 3 times
					Expect(failedDownloadCountEvtChannel).Should(HaveLen(3))
					Expect(failedDownloadCountEvtChannel).Should(Receive(Equal("http://xyz.example.com")))

					// Use Or() to combine matchers instead of the | operator
					Expect(loggerHook.LastEntry().Message).Should(Or(
						ContainSubstring("Can't download file:"),
						ContainSubstring("Name resolution err:")))
				})
		})
		When("a proxy is configured", func() {
			It("should be used", func(ctx context.Context) {
				proxy := TestHTTPProxy()

				sut.client.Transport = &http.Transport{Proxy: proxy.ReqURL}

				_, err := sut.DownloadFile(ctx, "http://example.com")
				Expect(err).Should(HaveOccurred())

				Expect(proxy.RequestTarget()).Should(Equal("example.com"))
			})
		})
	})
})
