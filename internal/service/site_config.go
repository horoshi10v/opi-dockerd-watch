package service

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var domainNamePattern = regexp.MustCompile(`(?i)^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func apacheVHostForSite(text string) (string, bool) {
	domain, ok := domainFromSite(text)
	if !ok {
		return "", false
	}

	return fmt.Sprintf(`<VirtualHost *:80>
    ServerName %[1]s
    ServerAlias www.%[1]s
    DocumentRoot /var/www/sites2/%[1]s

    Alias /__errors/ /var/www/_errors/

    <Directory /var/www/_errors/>
        Require all granted
    </Directory>

    ErrorDocument 404 /__errors/404.html

    <Directory /var/www/sites2/%[1]s>
        Options -Indexes +FollowSymLinks
        AllowOverride All
        Require all granted

        RewriteEngine On

        RewriteCond %%{THE_REQUEST} \s/{2,}
        RewriteRule ^ / [R=301,L]

        RewriteCond %%{REQUEST_URI} ^/$
        RewriteCond %%{QUERY_STRING} .
        RewriteRule ^ - [R=404,L]

        RewriteCond %%{THE_REQUEST} \s/+(.*/)?index\.html[\s?]
        RewriteRule ^(.*/)?index\.html$ /$1 [R=301,L]

        RewriteCond %%{REQUEST_FILENAME} -f
        RewriteRule ^ - [L]

        RewriteCond %%{REQUEST_FILENAME} -d
        RewriteRule ^ - [L]

        RewriteCond %%{REQUEST_URI} !^/__errors/
        RewriteRule ^ - [R=404,L]
    </Directory>

    ErrorLog ${APACHE_LOG_DIR}/%[1]s-error.log
    CustomLog ${APACHE_LOG_DIR}/%[1]s.log combined
</VirtualHost>`, domain), true
}

func domainFromSite(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" || strings.ContainsAny(text, "\r\n\t ") {
		return "", false
	}
	if !strings.Contains(text, "://") {
		text = "https://" + text
	}

	u, err := url.Parse(text)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return "", false
	}
	domain := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if !domainNamePattern.MatchString(domain) {
		return "", false
	}
	return domain, true
}
