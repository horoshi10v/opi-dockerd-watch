package service

import (
	"strings"
	"testing"
)

func TestApacheVHostForSite(t *testing.T) {
	config, ok := apacheVHostForSite("https://lamvuondedang.com/articles/?page=1")
	if !ok {
		t.Fatal("expected URL to be accepted")
	}

	for _, want := range []string{
		"ServerName lamvuondedang.com",
		"ServerAlias www.lamvuondedang.com",
		"DocumentRoot /var/www/sites2/lamvuondedang.com",
		"ErrorLog ${APACHE_LOG_DIR}/lamvuondedang.com-error.log",
		"RewriteCond %{THE_REQUEST} \\s/{2,}",
	} {
		if !strings.Contains(config, want) {
			t.Errorf("config does not contain %q", want)
		}
	}
}

func TestApacheVHostForSiteRejectsNonDomains(t *testing.T) {
	for _, input := range []string{"", "not a domain", "https://localhost", "ftp://example.com", "https://example.com@evil.test"} {
		if _, ok := apacheVHostForSite(input); ok {
			t.Errorf("expected %q to be rejected", input)
		}
	}
}
