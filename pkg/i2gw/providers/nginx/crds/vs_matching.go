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
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kubernetes-sigs/ingress2gateway/pkg/i2gw/notifications"
	nginxv1 "github.com/nginxinc/kubernetes-ingress/pkg/apis/configuration/v1"
)

// createAdvancedMatches processes VirtualServer matches and creates Gateway API HTTPRouteMatch
func createAdvancedMatches(vs nginxv1.VirtualServer, route nginxv1.Route, notifs *[]notifications.Notification) []gatewayv1.HTTPRouteMatch {
	var matches []gatewayv1.HTTPRouteMatch

	// Start with basic path match
	pathMatch := gatewayv1.HTTPRouteMatch{
		Path: &gatewayv1.HTTPPathMatch{
			Type:  Ptr(gatewayv1.PathMatchPathPrefix),
			Value: Ptr(route.Path),
		},
	}

	// If no advanced matches, return basic path match
	if len(route.Matches) == 0 {
		return []gatewayv1.HTTPRouteMatch{pathMatch}
	}

	// Process each match condition
	for _, match := range route.Matches {
		// Create a new match starting with path
		advancedMatch := pathMatch

		// Process header/cookie/query conditions
		if len(match.Conditions) > 0 {
			headerMatches, queryMatches := processConditions(match.Conditions, vs, notifs)
			
			if len(headerMatches) > 0 {
				advancedMatch.Headers = headerMatches
			}
			
			if len(queryMatches) > 0 {
				advancedMatch.QueryParams = queryMatches
			}
		}

		matches = append(matches, advancedMatch)
	}

	return matches
}

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
			// Convert cookie to header match (Cookie header)
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

	// Default to exact match
	matchType := gatewayv1.HeaderMatchExact

	// Check for regex patterns or wildcards in value
	if containsRegexPatterns(condition.Value) {
		matchType = gatewayv1.HeaderMatchRegularExpression
		addNotification(notifs, notifications.InfoNotification,
			"Header condition with regex pattern converted to RegularExpression match", &vs)
	}

	return &gatewayv1.HTTPHeaderMatch{
		Type:  Ptr(matchType),
		Name:  gatewayv1.HTTPHeaderName(condition.Header),
		Value: condition.Value,
	}
}

// createQueryMatch creates an HTTPQueryParamMatch from a condition  
func createQueryMatch(condition nginxv1.Condition, vs nginxv1.VirtualServer, notifs *[]notifications.Notification) *gatewayv1.HTTPQueryParamMatch {
	if condition.Argument == "" || condition.Value == "" {
		addNotification(notifs, notifications.WarningNotification,
			"Query parameter condition missing name or value", &vs)
		return nil
	}

	// Default to exact match
	matchType := gatewayv1.QueryParamMatchExact

	// Check for regex patterns
	if containsRegexPatterns(condition.Value) {
		matchType = gatewayv1.QueryParamMatchRegularExpression
		addNotification(notifs, notifications.InfoNotification,
			"Query parameter condition with regex pattern converted to RegularExpression match", &vs)
	}

	return &gatewayv1.HTTPQueryParamMatch{
		Type:  Ptr(matchType),
		Name:  gatewayv1.HTTPHeaderName(condition.Argument),
		Value: condition.Value,
	}
}

// createCookieMatch creates an HTTPHeaderMatch for Cookie header from a condition
func createCookieMatch(condition nginxv1.Condition, vs nginxv1.VirtualServer, notifs *[]notifications.Notification) *gatewayv1.HTTPHeaderMatch {
	if condition.Cookie == "" || condition.Value == "" {
		addNotification(notifs, notifications.WarningNotification,
			"Cookie condition missing name or value", &vs)
		return nil
	}

	// Convert cookie condition to Cookie header match
	// Cookie: name=value format
	cookiePattern := condition.Cookie + "=" + condition.Value

	addNotification(notifs, notifications.InfoNotification,
		"Cookie condition converted to Cookie header match", &vs)

	return &gatewayv1.HTTPHeaderMatch{
		Type:  Ptr(gatewayv1.HeaderMatchRegularExpression),
		Name:  gatewayv1.HTTPHeaderName("Cookie"),
		Value: ".*" + cookiePattern + ".*", // Match cookie anywhere in Cookie header
	}
}

// containsRegexPatterns checks if a value contains regex special characters
func containsRegexPatterns(value string) bool {
	regexChars := []string{"*", "^", "$", "[", "]", "(", ")", ".", "+", "?", "|", "\\"}
	for _, char := range regexChars {
		if len(value) > 0 && (value[0:1] == char || value[len(value)-1:] == char || 
			len(value) > 2 && value[1:len(value)-1] != value) {
			return true
		}
	}
	return false
}