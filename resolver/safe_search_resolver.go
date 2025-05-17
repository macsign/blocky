package resolver

import (
	"context"

	"github.com/0xERR0R/blocky/config"
	"github.com/0xERR0R/blocky/model"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
)

type SafeSearchResolver struct {
	configurable[*config.SafeSearchConfig]
	NextResolver
	typed
}

// IsEnabled implements Resolver.
// Subtle: this method shadows the method (configurable).IsEnabled of SafeSearchResolver.configurable.
func (s *SafeSearchResolver) IsEnabled() bool {
	panic("unimplemented")
}

// LogConfig implements Resolver.
// Subtle: this method shadows the method (configurable).LogConfig of SafeSearchResolver.configurable.
func (s *SafeSearchResolver) LogConfig(*logrus.Entry) {
	panic("unimplemented")
}

// Resolve implements Resolver.
func (s *SafeSearchResolver) Resolve(ctx context.Context, req *model.Request) (*model.Response, error) {
	response := new(dns.Msg)
	response.Answer = s.LookupSearchEngine(req.Req.Question)
	return &model.Response{Res: response, RType: model.ResponseTypeSAFESEARCH, Reason: "CACHED NEGATIVE"}, nil
}

func (s *SafeSearchResolver) LookupSearchEngine(questions []dns.Question) []dns.RR {
	answers := []dns.RR{}
	for _, question := range questions {
		domain := question.Name
		for _, engine := range s.cfg.SearchEngines {
			if dns.Fqdn(engine.Domain) == dns.Fqdn(domain) {
				cname := new(dns.CNAME)
				cname.Hdr = dns.RR_Header{Class: dns.ClassINET, Ttl: 3600, Rrtype: dns.TypeCNAME, Name: domain}
				cname.Target = dns.Fqdn(engine.SafeSearchCname)
				answers = append(answers, cname)
			}
		}
	}
	return answers
}

// String implements Resolver.
// Subtle: this method shadows the method (typed).String of SafeSearchResolver.typed.
func (s *SafeSearchResolver) String() string {
	panic("unimplemented")
}

// Type implements Resolver.
// Subtle: this method shadows the method (typed).Type of SafeSearchResolver.typed.
func (s *SafeSearchResolver) Type() string {
	return "safe_search"
}

func NewSafeSearchResolver(cfg config.SafeSearchConfig) *SafeSearchResolver {
	cfg.SetDefaults()
	return &SafeSearchResolver{
		configurable: withConfig(&cfg),
		typed:        withType("safe_search"),
	}
}
