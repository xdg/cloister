//go:build e2e

package e2e

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
)

// TestDomainDenial_GlobalScope verifies that denying a domain with scope="global"
// persists the denial to the global decisions file and blocks subsequent requests
// immediately (no re-prompt).
//
// Flow:
// 1. Container makes a proxy request to an unlisted domain (blocks in approval queue)
// 2. Test denies the domain with scope="global" via the approval server API
// 3. Proxy request completes with 403
// 4. Verify denied_domains in global decisions file includes the domain
// 5. Make a second request to the same domain and verify immediate 403 (no approval prompt)
func TestDomainDenial_GlobalScope(t *testing.T) {
	tc := createAuthenticatedTestContainer(t, "deny-global")
	guardianHost := guardian.ContainerName()
	port := approvalPort(t)

	// Wait for proxy to be ready
	if err := waitForPort(t, tc.Name, guardianHost, 3128, 5*time.Second); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	testDomain := "global-deny-test.example.com"

	// First request: will block in approval queue
	var wg sync.WaitGroup
	var proxyOutput string
	var proxyErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		proxyOutput, proxyErr = execInContainer(t, tc.Name,
			"sh", "-c",
			"curl -v --proxy http://"+guardianHost+":3128 --proxy-user :"+tc.Token+
				" --max-time 15 https://"+testDomain+"/ 2>&1 | grep -oE 'HTTP/[0-9.]+ [0-9]+' | head -1 | awk '{print $2}'")
	}()

	// Wait for the domain request to appear, then deny it with global scope
	requestID := waitForPendingDomain(t, port, testDomain, 10*time.Second)
	t.Logf("Found pending domain request: id=%s", requestID)
	denyDomain(t, port, requestID, "global", false)
	t.Log("Denied domain with global scope")

	wg.Wait()

	if proxyErr != nil && strings.Contains(proxyOutput, "not found") {
		t.Skip("curl not available in container image")
	}

	// First request should have been denied (403)
	trimmedOutput := strings.TrimSpace(proxyOutput)
	if trimmedOutput != "403" {
		t.Errorf("Expected first request to get 403, got: %q", trimmedOutput)
	}

	// Verify the domain is persisted in the global decisions file
	decisions, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("Failed to load global decisions: %v", err)
	}
	found := false
	for _, d := range decisions.DeniedDomains {
		if d == testDomain || d == testDomain+":443" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected domain %q in global decisions denied_domains, got: %v",
			testDomain, decisions.DeniedDomains)
	}

	// Give the guardian a moment to reload after config write
	time.Sleep(500 * time.Millisecond)

	// Second request: should be immediately blocked (403, no approval prompt)
	startTime := time.Now()
	output2, _ := execInContainer(t, tc.Name,
		"sh", "-c",
		"curl -v --proxy http://"+guardianHost+":3128 --proxy-user :"+tc.Token+
			" --max-time 8 https://"+testDomain+"/ 2>&1 | grep -oE 'HTTP/[0-9.]+ [0-9]+' | head -1 | awk '{print $2}'")
	elapsed := time.Since(startTime)

	output2 = strings.TrimSpace(output2)
	if output2 != "403" {
		t.Errorf("Expected second request to get 403 (persisted denial), got: %q", output2)
	}

	// If the response took more than 2.5s, the domain was probably re-queued for approval
	// rather than immediately blocked. The approval timeout is 3s, so a fast response
	// means the deny was applied from the persisted decisions file.
	if elapsed > 2500*time.Millisecond {
		t.Errorf("Second request took %v; expected fast 403 for denied domain (approval timeout is 3s)", elapsed)
	}
}

// TestDomainDenial_SessionScope_BlocksSubsequentRequests verifies that denying a
// domain with scope="session" blocks subsequent requests in the same session (403).
//
// Note: A full restart test is out of scope for a single test (would need to restart
// the guardian). The session denylist is memory-only (MemorySessionDenylist), so
// denials are automatically forgotten when the guardian restarts.
//
// Flow:
// 1. Container makes a proxy request to an unlisted domain (blocks in approval queue)
// 2. Test denies the domain with scope="session"
// 3. Proxy request completes with 403
// 4. Make a second request to verify it is also blocked (403)
func TestDomainDenial_SessionScope_BlocksSubsequentRequests(t *testing.T) {
	tc := createAuthenticatedTestContainer(t, "deny-session")
	guardianHost := guardian.ContainerName()
	port := approvalPort(t)

	// Wait for proxy to be ready
	if err := waitForPort(t, tc.Name, guardianHost, 3128, 5*time.Second); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	testDomain := "session-deny-test.example.com"

	// First request: will block in approval queue
	var wg sync.WaitGroup
	var proxyOutput string
	var proxyErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		proxyOutput, proxyErr = execInContainer(t, tc.Name,
			"sh", "-c",
			"curl -v --proxy http://"+guardianHost+":3128 --proxy-user :"+tc.Token+
				" --max-time 15 https://"+testDomain+"/ 2>&1 | grep -oE 'HTTP/[0-9.]+ [0-9]+' | head -1 | awk '{print $2}'")
	}()

	// Wait for the domain request to appear, then deny it with session scope
	requestID := waitForPendingDomain(t, port, testDomain, 10*time.Second)
	t.Logf("Found pending domain request: id=%s", requestID)
	denyDomain(t, port, requestID, "session", false)
	t.Log("Denied domain with session scope")

	wg.Wait()

	if proxyErr != nil && strings.Contains(proxyOutput, "not found") {
		t.Skip("curl not available in container image")
	}

	// First request should have been denied (403)
	trimmedOutput := strings.TrimSpace(proxyOutput)
	if trimmedOutput != "403" {
		t.Errorf("Expected first request to get 403, got: %q", trimmedOutput)
	}

	// Give the guardian a moment to process the session denial
	time.Sleep(200 * time.Millisecond)

	// Second request: should be immediately blocked by session denylist (403)
	startTime := time.Now()
	output2, _ := execInContainer(t, tc.Name,
		"sh", "-c",
		"curl -v --proxy http://"+guardianHost+":3128 --proxy-user :"+tc.Token+
			" --max-time 8 https://"+testDomain+"/ 2>&1 | grep -oE 'HTTP/[0-9.]+ [0-9]+' | head -1 | awk '{print $2}'")
	elapsed := time.Since(startTime)

	output2 = strings.TrimSpace(output2)
	if output2 != "403" {
		t.Errorf("Expected second request to get 403 (session denial), got: %q", output2)
	}

	// If the response took more than 2.5s, the domain was probably re-queued for approval
	// rather than immediately blocked from the session denylist.
	if elapsed > 2500*time.Millisecond {
		t.Errorf("Second request took %v; expected fast 403 for session-denied domain", elapsed)
	}

	// Session denials should NOT be persisted to disk. Verify the global decisions
	// file does not contain this domain.
	decisions, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("Failed to load global decisions: %v", err)
	}
	for _, d := range decisions.DeniedDomains {
		if d == testDomain || d == testDomain+":443" {
			t.Errorf("Session-scoped denial should NOT be persisted to global decisions, but found %q", d)
		}
	}
}

// TestDomainApproval_OnceScope_RePrompts verifies that approving a domain with
// scope="once" does NOT persist the approval, so a second request to the same
// domain re-enters the approval queue.
//
// Flow:
// 1. Container makes a proxy request to an unlisted domain (blocks in approval queue)
// 2. Test approves the domain with scope="once"
// 3. Proxy request completes (approved)
// 4. Make a second request to the same domain
// 5. Verify the domain appears again in the pending approval queue (re-prompts)
func TestDomainApproval_OnceScope_RePrompts(t *testing.T) {
	tc := createAuthenticatedTestContainer(t, "once-reprompt")
	guardianHost := guardian.ContainerName()
	port := approvalPort(t)

	// Wait for proxy to be ready
	if err := waitForPort(t, tc.Name, guardianHost, 3128, 5*time.Second); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	testDomain := "once-reprompt-test.example.com"

	// First request: will block in approval queue
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		output, _ := execInContainer(t, tc.Name,
			"sh", "-c",
			"curl -v --proxy http://"+guardianHost+":3128 --proxy-user :"+tc.Token+
				" --max-time 15 https://"+testDomain+"/ 2>&1")
		if strings.Contains(output, "not found") {
			t.Log("curl not available in container image")
		}
	}()

	// Wait for the first approval request, then approve with "once" scope
	requestID := waitForPendingDomain(t, port, testDomain, 10*time.Second)
	t.Logf("Found first pending domain request: id=%s", requestID)
	approveDomain(t, port, requestID, "once")
	t.Log("Approved domain with once scope")

	wg.Wait()

	// Brief pause to let the proxy fully process the first request
	time.Sleep(300 * time.Millisecond)

	// Second request: should re-enter the approval queue (scope=once is not persisted)
	wg.Add(1)
	go func() {
		defer wg.Done()
		// This request will block waiting for approval (or timeout)
		_, _ = execInContainer(t, tc.Name,
			"sh", "-c",
			"curl -v --proxy http://"+guardianHost+":3128 --proxy-user :"+tc.Token+
				" --max-time 15 https://"+testDomain+"/ 2>&1")
	}()

	// Verify the domain appears again in the pending queue (re-prompted)
	requestID2 := waitForPendingDomain(t, port, testDomain, 10*time.Second)
	t.Logf("Found second pending domain request (re-prompted): id=%s", requestID2)

	if requestID2 == requestID {
		t.Error("Second request should have a different request ID (new approval request)")
	}

	// Approve the second request to let the goroutine finish cleanly
	approveDomain(t, port, requestID2, "once")
	wg.Wait()
}

// TestDomainDenial_Wildcard verifies that denying a domain with wildcard=true
// blocks all subdomains of the same parent. For example, denying
// "api.evil-test.example.com" with wildcard=true persists "*.evil-test.example.com"
// and blocks requests to "other.evil-test.example.com".
//
// Flow:
// 1. Container makes a proxy request to api.evil-test.example.com
// 2. Test denies with scope="global", wildcard=true
// 3. Verify denied_patterns in global decisions contains "*.evil-test.example.com"
// 4. Make a request to other.evil-test.example.com
// 5. Verify immediate 403 (wildcard pattern match)
func TestDomainDenial_Wildcard(t *testing.T) {
	tc := createAuthenticatedTestContainer(t, "deny-wildcard")
	guardianHost := guardian.ContainerName()
	port := approvalPort(t)

	// Wait for proxy to be ready
	if err := waitForPort(t, tc.Name, guardianHost, 3128, 5*time.Second); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	// Use a domain with enough levels for wildcard to apply
	// domainToWildcard requires at least 3 components (api.evil-test.example.com -> *.evil-test.example.com)
	testDomain := "api.evil-test.example.com"
	siblingDomain := "other.evil-test.example.com"
	expectedPattern := "*.evil-test.example.com"

	// First request: trigger approval queue for the test domain
	var wg sync.WaitGroup
	var proxyOutput string
	var proxyErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		proxyOutput, proxyErr = execInContainer(t, tc.Name,
			"sh", "-c",
			"curl -v --proxy http://"+guardianHost+":3128 --proxy-user :"+tc.Token+
				" --max-time 15 https://"+testDomain+"/ 2>&1 | grep -oE 'HTTP/[0-9.]+ [0-9]+' | head -1 | awk '{print $2}'")
	}()

	// Wait for the domain request, then deny with wildcard
	requestID := waitForPendingDomain(t, port, testDomain, 10*time.Second)
	t.Logf("Found pending domain request: id=%s", requestID)
	denyDomain(t, port, requestID, "global", true)
	t.Log("Denied domain with global scope and wildcard=true")

	wg.Wait()

	if proxyErr != nil && strings.Contains(proxyOutput, "not found") {
		t.Skip("curl not available in container image")
	}

	// First request should be denied
	trimmedOutput := strings.TrimSpace(proxyOutput)
	if trimmedOutput != "403" {
		t.Errorf("Expected first request to get 403, got: %q", trimmedOutput)
	}

	// Verify the wildcard pattern is in the global decisions file
	decisions, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("Failed to load global decisions: %v", err)
	}
	foundPattern := false
	for _, p := range decisions.DeniedPatterns {
		if p == expectedPattern {
			foundPattern = true
			break
		}
	}
	if !foundPattern {
		t.Errorf("Expected pattern %q in global decisions denied_patterns, got: %v",
			expectedPattern, decisions.DeniedPatterns)
	}

	// Give the guardian a moment to reload after config write
	time.Sleep(500 * time.Millisecond)

	// Second request to a sibling subdomain: should be blocked by wildcard pattern
	startTime := time.Now()
	output2, _ := execInContainer(t, tc.Name,
		"sh", "-c",
		"curl -v --proxy http://"+guardianHost+":3128 --proxy-user :"+tc.Token+
			" --max-time 8 https://"+siblingDomain+"/ 2>&1 | grep -oE 'HTTP/[0-9.]+ [0-9]+' | head -1 | awk '{print $2}'")
	elapsed := time.Since(startTime)

	output2 = strings.TrimSpace(output2)
	if output2 != "403" {
		t.Errorf("Expected sibling domain request to get 403 (wildcard denial), got: %q", output2)
	}

	// A fast response confirms the wildcard denial was applied from the cache/config
	// rather than timing out in the approval queue.
	if elapsed > 2500*time.Millisecond {
		t.Errorf("Sibling domain request took %v; expected fast 403 for wildcard-denied domain", elapsed)
	}
}

// TestDomainDenial_DenyWinsOverAllow verifies that when a domain appears in both
// the allowed and denied lists, the denial takes precedence (deny wins over allow).
//
// This test writes a domain to the project decisions file with both domains
// (allowed) and denied_domains fields, sends SIGHUP to reload, and verifies
// that the proxy returns 403.
func TestDomainDenial_DenyWinsOverAllow(t *testing.T) {
	tc := createAuthenticatedTestContainer(t, "deny-wins")
	guardianHost := guardian.ContainerName()

	// Wait for proxy to be ready
	if err := waitForPort(t, tc.Name, guardianHost, 3128, 5*time.Second); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	testDomain := "deny-wins-test.example.com"

	// Write a project decisions file that has the domain in BOTH allowed and denied lists.
	// The token was registered with project "test-project" by createAuthenticatedTestContainer.
	decisions := &config.Decisions{
		Domains:       []string{testDomain},
		DeniedDomains: []string{testDomain},
	}
	if err := config.WriteProjectDecisions("test-project", decisions); err != nil {
		t.Fatalf("Failed to write project decisions: %v", err)
	}
	t.Cleanup(func() {
		// Clean up the decisions file after the test
		_ = os.Remove(config.ProjectDecisionPath("test-project"))
	})

	// Send SIGHUP to the guardian container to trigger config reload
	_, err := docker.Run("kill", "-s", "HUP", guardian.ContainerName())
	if err != nil {
		t.Fatalf("Failed to send SIGHUP to guardian: %v", err)
	}

	// Wait for the reload to take effect
	time.Sleep(1 * time.Second)

	// Make a proxy request to the domain that is both allowed and denied
	startTime := time.Now()
	output, _ := execInContainer(t, tc.Name,
		"sh", "-c",
		"curl -v --proxy http://"+guardianHost+":3128 --proxy-user :"+tc.Token+
			" --max-time 8 https://"+testDomain+"/ 2>&1 | grep -oE 'HTTP/[0-9.]+ [0-9]+' | head -1 | awk '{print $2}'")
	elapsed := time.Since(startTime)

	if strings.Contains(output, "not found") {
		t.Skip("curl not available in container image")
	}

	output = strings.TrimSpace(output)

	// Deny should win over allow: expect 403
	if output != "403" {
		t.Errorf("Expected 403 (deny wins over allow), got: %q", output)
	}

	// A fast response confirms the denial was applied from the static denylist
	// rather than going through the approval queue.
	if elapsed > 2500*time.Millisecond {
		t.Errorf("Request took %v; expected fast 403 for statically denied domain", elapsed)
	}
}

// TestDomainDenial_LoadDecisionsOnStartup verifies that denied domains from
// the decisions file are applied after a config reload (simulating startup behavior).
//
// Since the guardian is already running (managed by TestMain), this test writes
// a denied domain to the global decisions file, sends SIGHUP to trigger a reload,
// and verifies that the domain is immediately blocked.
func TestDomainDenial_LoadDecisionsOnStartup(t *testing.T) {
	tc := createAuthenticatedTestContainer(t, "deny-startup")
	guardianHost := guardian.ContainerName()

	// Wait for proxy to be ready
	if err := waitForPort(t, tc.Name, guardianHost, 3128, 5*time.Second); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	testDomain := "startup-denied.example.com"

	// Load existing global decisions and add our test domain to the denied list,
	// preserving any domains already written by previous tests.
	existing, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("Failed to load existing global decisions: %v", err)
	}
	existing.DeniedDomains = append(existing.DeniedDomains, testDomain)
	if err := config.WriteGlobalDecisions(existing); err != nil {
		t.Fatalf("Failed to write global decisions: %v", err)
	}

	// Send SIGHUP to the guardian container to trigger config reload
	_, err = docker.Run("kill", "-s", "HUP", guardian.ContainerName())
	if err != nil {
		t.Fatalf("Failed to send SIGHUP to guardian: %v", err)
	}

	// Wait for the reload to take effect
	time.Sleep(1 * time.Second)

	// Make a proxy request to the pre-denied domain
	startTime := time.Now()
	output, _ := execInContainer(t, tc.Name,
		"sh", "-c",
		"curl -v --proxy http://"+guardianHost+":3128 --proxy-user :"+tc.Token+
			" --max-time 8 https://"+testDomain+"/ 2>&1 | grep -oE 'HTTP/[0-9.]+ [0-9]+' | head -1 | awk '{print $2}'")
	elapsed := time.Since(startTime)

	if strings.Contains(output, "not found") {
		t.Skip("curl not available in container image")
	}

	output = strings.TrimSpace(output)

	// Domain should be blocked immediately from the loaded decisions file
	if output != "403" {
		t.Errorf("Expected 403 for pre-denied domain loaded from decisions file, got: %q", output)
	}

	// A fast response confirms the denial was applied from the reloaded config
	// rather than entering the approval queue.
	if elapsed > 2500*time.Millisecond {
		t.Errorf("Request took %v; expected fast 403 for pre-denied domain (approval timeout is 3s)", elapsed)
	}
}
