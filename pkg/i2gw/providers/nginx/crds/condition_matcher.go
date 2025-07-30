/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package crds

import (
	"fmt"
	"regexp"
	"strings"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	nginxv1 "github.com/nginx/kubernetes-ingress/pkg/apis/configuration/v1"
)

// processConditions converts VirtualServer conditions to Gateway API matches
func processConditions(conditions []nginxv1.Condition, vs nginxv1.VirtualServer, notifs *[]notifications.Notification) ([]gatewayv1.HTTPHeaderMatch, []gatewayv1.HTTPQueryParamMatch) {
	var headerMatches []gatewayv1.HTTPHeaderMatch
	var queryMatches []gatewayv1.HTTPQueryParamMatch

	for _, condition := range conditions {
		switch {
		case condition.Header != "":
			headerMatch := createHeaderMatch(condition, vs, notifs)
			if headerMatch != nil {
				headerMatches = append(headerMatches, *headerMatch)
			}

		case condition.Argument != "":
			queryMatch := createQueryMatch(condition, vs, notifs)
			if queryMatch != nil {
				queryMatches = append(queryMatches, *queryMatch)
			}

		case condition.Cookie != "":
			cookieMatch := createCookieMatch(condition, vs, notifs)
			if cookieMatch != nil {
				headerMatches = append(headerMatches, *cookieMatch)
			}

		case condition.Variable != "":
			// NGINX variables are not directly supported in Gateway API
			addNotification(notifs, notifications.InfoNotification,
				"NGINX variable condition stored in provider-specific IR - not directly supported in Gateway API", &vs)
		}
	}

	return headerMatches, queryMatches
}

// createHeaderMatch creates an HTTPHeaderMatch from a condition
func createHeaderMatch(condition nginxv1.Condition, vs nginxv1.VirtualServer, notifs *[]notifications.Notification) *gatewayv1.HTTPHeaderMatch {
	if condition.Header == "" || condition.Value == "" {
		addNotification(notifs, notifications.WarningNotification,
			"Header condition missing name or value", &vs)
		return nil
	}
	raw := condition.Value
	negate := false

	if strings.HasPrefix(raw, "!") {
		negate = true
		raw = raw[1:]
	}
	pattern := raw

	// If it's not already a regex, quote and wrap for case‑insensitive exact match
	if !containsRegexPatterns(pattern) {
		escaped := regexp.QuoteMeta(pattern)
		pattern = fmt.Sprintf("(?i)^%s$", escaped)
	}

	// If negated, wrap in a negative lookahead
	if negate {
		pattern = fmt.Sprintf("^(?!%s).*$", pattern)
	}

	return &gatewayv1.HTTPHeaderMatch{
		Type:  Ptr(gatewayv1.HeaderMatchRegularExpression),
		Name:  gatewayv1.HTTPHeaderName(condition.Header),
		Value: pattern,
	}
}

// createQueryMatch creates an HTTPQueryParamMatch from a condition
func createQueryMatch(condition nginxv1.Condition, vs nginxv1.VirtualServer, notifs *[]notifications.Notification) *gatewayv1.HTTPQueryParamMatch {
	if condition.Argument == "" || condition.Value == "" {
		addNotification(notifs, notifications.WarningNotification,
			"Query parameter condition missing name or value", &vs)
		return nil
	}

	raw := condition.Value
	negate := false

	if strings.HasPrefix(raw, "!") {
		negate = true
		raw = raw[1:]
	}
	pattern := raw

	// If it's not already a regex, quote and wrap for case‑insensitive exact match
	if !containsRegexPatterns(pattern) {
		escaped := regexp.QuoteMeta(pattern)
		pattern = fmt.Sprintf("(?i)^%s$", escaped)
	}

	// If negated, wrap in a negative lookahead
	if negate {
		pattern = fmt.Sprintf("^(?!%s).*$", pattern)
	}

	return &gatewayv1.HTTPQueryParamMatch{
		Type:  Ptr(gatewayv1.QueryParamMatchRegularExpression),
		Name:  gatewayv1.HTTPHeaderName(condition.Argument),
		Value: pattern,
	}
}

// createCookieMatch creates an HTTPHeaderMatch for Cookie header from a condition
func createCookieMatch(condition nginxv1.Condition, vs nginxv1.VirtualServer, notifs *[]notifications.Notification) *gatewayv1.HTTPHeaderMatch {
	if condition.Cookie == "" || condition.Value == "" {
		addNotification(notifs, notifications.WarningNotification,
			"Cookie condition missing name or value", &vs)
		return nil
	}

	raw := condition.Value
	negate := false

	if strings.HasPrefix(raw, "!") {
		negate = true
		raw = raw[1:]
	}

	// Create cookie pattern: cookiename=value
	cookieNameValue := condition.Cookie + "=" + raw
	pattern := cookieNameValue

	// If it's not already a regex, quote and wrap for case-insensitive match within Cookie header
	if !containsRegexPatterns(pattern) {
		escaped := regexp.QuoteMeta(pattern)
		pattern = fmt.Sprintf("(?i).*\\b%s\\b.*", escaped)
	} else {
		// If it's a regex, wrap to match anywhere in Cookie header
		pattern = fmt.Sprintf("(?i).*%s.*", pattern)
	}

	// If negated, wrap in a negative lookahead
	if negate {
		pattern = fmt.Sprintf("^(?!.*%s).*$", cookieNameValue)
	}

	addNotification(notifs, notifications.InfoNotification,
		"Cookie condition converted to Cookie header match", &vs)

	return &gatewayv1.HTTPHeaderMatch{
		Type:  Ptr(gatewayv1.HeaderMatchRegularExpression),
		Name:  gatewayv1.HTTPHeaderName("Cookie"),
		Value: pattern,
	}
}

// containsRegexPatterns is implemented in utils.go
