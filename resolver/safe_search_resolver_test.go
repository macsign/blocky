package resolver

import (
	"context"

	"github.com/0xERR0R/blocky/config"
	. "github.com/0xERR0R/blocky/helpertest"
	. "github.com/0xERR0R/blocky/model"
	"github.com/miekg/dns"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("SafeSearchResolver", func() {
	var (
		safeSearchResolver *SafeSearchResolver
		safeSearchConfig   config.SafeSearchConfig
		nextResolverMock   *mockResolver
		ctx                context.Context
		clientId           string
		clientGroups       map[string][]string
	)

	BeforeEach(func() {
		clientGroups = map[string][]string{
			"default": {"google", "bing", "brave"},
		}
		clientId = "client.home"
	})

	JustBeforeEach(func() {
		safeSearchConfig = config.SafeSearchConfig{ClientGroups: clientGroups}
		safeSearchResolver = NewSafeSearchResolver(safeSearchConfig)
		ctx, _ = context.WithCancel(context.Background())
		nextResolverMock = &mockResolver{}
		nextResolverMock.On("Resolve", mock.Anything).Return(&Response{RType: ResponseTypeRESOLVED, Res: new(dns.Msg), Reason: "reason"}, nil)
		safeSearchResolver.Next(nextResolverMock)
	})

	Describe("Type", func() {
		It("follows conventions", func() {
			expectValidResolverType(safeSearchResolver)
		})
	})

	Describe("Enforce safe search if applies", func() {
		When("client group is configured", func() {
			It("should set cname if domain belongs to a configured search engine A", func() {
				Expect(safeSearchResolver.Resolve(ctx, newRequestWithClientID("google.com.", A, "::1", clientId))).
					Should(SatisfyAll(
						HaveResponseType(ResponseTypeSAFESEARCH),
						BeDNSRecord("google.com.", CNAME, "forcesafesearch.google.com."),
					))
				Expect(nextResolverMock.Calls).Should(BeEmpty())
			})
			It("should set cname if domain belongs to a configured search engine AAAA", func() {
				Expect(safeSearchResolver.Resolve(ctx, newRequestWithClientID("google.com.", AAAA, "::1", clientId))).
					Should(SatisfyAll(
						HaveResponseType(ResponseTypeSAFESEARCH),
						BeDNSRecord("google.com.", CNAME, "forcesafesearch.google.com."),
					))
				Expect(nextResolverMock.Calls).Should(BeEmpty())
			})
			It("should set cname if domain belongs to a configured search engine CNAME", func() {
				Expect(safeSearchResolver.Resolve(ctx, newRequestWithClientID("google.com.", CNAME, "::1", clientId))).
					Should(SatisfyAll(
						HaveResponseType(ResponseTypeSAFESEARCH),
						BeDNSRecord("google.com.", CNAME, "forcesafesearch.google.com."),
					))
				Expect(nextResolverMock.Calls).Should(BeEmpty())
			})
			It("should not set cname if domain belongs to a configured search engine HTTPS", func() {
				Expect(safeSearchResolver.Resolve(ctx, newRequestWithClientID("google.com.", HTTPS, "::1", clientId))).
					Should(SatisfyAll(
						HaveResponseType(ResponseTypeSAFESEARCH),
						BeDNSRecord("google.com.", CNAME, "forcesafesearch.google.com."),
					))
				Expect(nextResolverMock.Calls).Should(HaveLen(1))
			})
			It("should not set cname if domain belongs to a configured search engine PTR", func() {
				Expect(safeSearchResolver.Resolve(ctx, newRequestWithClientID("google.com.", PTR, "::1", clientId))).
					Should(SatisfyAll(
						HaveResponseType(ResponseTypeRESOLVED),
						BeDNSRecord("google.com.", CNAME, "forcesafesearch.google.com."),
					))
				Expect(nextResolverMock.Calls).Should(HaveLen(1))
			})
		})
	})
})
