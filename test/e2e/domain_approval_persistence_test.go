//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/executor"
	"github.com/xdg/cloister/internal/guardian"
)

// approvalPort returns the approval server port from executor state.
// The approval server is published on localhost by the guardian container.
func approvalPort(t *testing.T) int {
	t.Helper()
	state, err := executor.LoadDaemonState()
	if err != nil {
		t.Fatalf("Failed to load executor daemon state: %v", err)
	}
	if state == nil || state.ApprovalPort == 0 {
		t.Fatal("Approval port not found in executor daemon state")
	}
	return state.ApprovalPort
}

// pendingDomainJSON represents a single pending domain request from the API.
type pendingDomainJSON struct {
	ID        string `json:"id"`
	Cloister  string `json:"cloister"`
	Project   string `json:"project"`
	Domain    string `json:"domain"`
	Timestamp string `json:"timestamp"`
}

// pendingDomainsJSON is the response from GET /pending-domains.
type pendingDomainsJSON struct {
	Requests []pendingDomainJSON `json:"requests"`
}

// waitForPendingDomain polls the approval server for a pending domain request
// matching the given domain. Returns the request ID when found.
func waitForPendingDomain(t *testing.T, port int, domain string, timeout time.Duration) string {
	t.Helper()

	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/pending-domains", port)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck // best-effort close in polling loop
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		var pending pendingDomainsJSON
		if err := json.Unmarshal(body, &pending); err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		for _, req := range pending.Requests {
			// Match on the domain (which includes port in CONNECT requests)
			if req.Domain == domain || strings.HasPrefix(req.Domain, domain+":") {
				return req.ID
			}
		}

		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf("Timed out waiting for pending domain request for %s", domain)
	return ""
}

// approveDomain sends an approval for a pending domain request.
func approveDomain(t *testing.T, port int, requestID, scope string) {
	t.Helper()

	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/approve-domain/%s", port, requestID)

	body := fmt.Sprintf(`{"scope": %q}`, scope)
	resp, err := client.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to approve domain: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // test helper, always followed by ReadAll

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Approve domain returned status %d: %s", resp.StatusCode, string(respBody))
	}
}

// TestDomainApprovalPersistence_ProjectScope verifies that approving a domain
// with "project" scope persists to the project approval file on disk.
//
// Flow:
// 1. Container makes a proxy request to an unlisted domain (blocks in approval queue)
// 2. Test approves the domain with "project" scope via the approval server API
// 3. Proxy request completes (approved)
// 4. Verify approval file written at approvals/projects/test-project.yaml
// 5. Verify static config untouched
func TestDomainApprovalPersistence_ProjectScope(t *testing.T) {
	tc := createAuthenticatedTestContainer(t, "approval-proj")
	guardianHost := guardian.ContainerName()
	port := approvalPort(t)

	// Wait for proxy to be ready
	if err := waitForPort(t, tc.Name, guardianHost, 3128, 5*time.Second); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	// Record the static project config state before the test.
	// The token was registered with project "test-project" by createAuthenticatedTestContainer.
	projectConfigPath := config.ProjectConfigPath("test-project")
	staticConfigBefore, _ := os.ReadFile(projectConfigPath)

	// Use a unique unlisted domain for this test
	testDomain := "project-approval-test.example.com"

	// Launch the proxy request in a goroutine (it blocks waiting for approval)
	var wg sync.WaitGroup
	var proxyOutput string
	var proxyErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Make a CONNECT request through the proxy to the unlisted domain.
		// Use verbose output to capture the HTTP response code.
		proxyOutput, proxyErr = execInContainer(t, tc.Name,
			"sh", "-c",
			"curl -v --proxy http://"+guardianHost+":3128 --proxy-user :"+tc.Token+
				" --max-time 15 https://"+testDomain+"/ 2>&1 | grep -oE 'HTTP/[0-9.]+ [0-9]+' | head -1 | awk '{print $2}'")
	}()

	// Wait for the domain approval request to appear in the queue
	requestID := waitForPendingDomain(t, port, testDomain, 10*time.Second)
	t.Logf("Found pending domain request: id=%s", requestID)

	// Approve with project scope
	approveDomain(t, port, requestID, "project")
	t.Log("Approved domain with project scope")

	// Wait for the proxy request goroutine to complete
	wg.Wait()

	if proxyErr != nil && strings.Contains(proxyOutput, "not found") {
		t.Skip("curl not available in container image")
	}

	// The proxy request was approved, so CONNECT should have succeeded (200).
	// However, the upstream domain doesn't actually resolve, so curl may show
	// a connection error after the CONNECT succeeds. We just verify the CONNECT
	// was not 403 (which would mean the approval didn't work).
	trimmedOutput := strings.TrimSpace(proxyOutput)
	if trimmedOutput == "403" {
		t.Errorf("Expected CONNECT to be approved (not 403), got: %q", trimmedOutput)
	}

	// Verify the decision file was created on disk.
	// The guardian container writes to the decision dir, which is mounted rw
	// at /etc/cloister/decisions (overlaying the ro config mount).
	projectApprovalPath := config.ProjectDecisionPath("test-project")
	approvals, err := config.LoadProjectDecisions("test-project")
	if err != nil {
		t.Fatalf("Failed to load project approvals: %v", err)
	}

	// Check that the test domain is in the approved list
	found := false
	for _, d := range approvals.Domains {
		if d == testDomain || d == testDomain+":443" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected domain %q in project approvals at %s, got domains: %v",
			testDomain, projectApprovalPath, approvals.Domains)
	}

	// Verify static project config was NOT modified
	staticConfigAfter, _ := os.ReadFile(projectConfigPath)
	if string(staticConfigBefore) != string(staticConfigAfter) {
		t.Error("Static project config was modified; expected it to remain unchanged")
	}
}

// TestDomainApprovalPersistence_GlobalScope verifies that approving a domain
// with "global" scope persists to the global approval file on disk.
//
// Flow:
// 1. Container makes a proxy request to an unlisted domain (blocks in approval queue)
// 2. Test approves the domain with "global" scope via the approval server API
// 3. Proxy request completes (approved)
// 4. Verify approval file written at approvals/global.yaml
// 5. Verify static global config untouched
func TestDomainApprovalPersistence_GlobalScope(t *testing.T) {
	tc := createAuthenticatedTestContainer(t, "approval-glob")
	guardianHost := guardian.ContainerName()
	port := approvalPort(t)

	// Wait for proxy to be ready
	if err := waitForPort(t, tc.Name, guardianHost, 3128, 5*time.Second); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	// Record the static global config state before the test.
	globalConfigPath := config.GlobalConfigPath()
	staticConfigBefore, _ := os.ReadFile(globalConfigPath)

	// Use a unique unlisted domain for this test
	testDomain := "global-approval-test.example.com"

	// Launch the proxy request in a goroutine (it blocks waiting for approval)
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

	// Wait for the domain approval request to appear in the queue
	requestID := waitForPendingDomain(t, port, testDomain, 10*time.Second)
	t.Logf("Found pending domain request: id=%s", requestID)

	// Approve with global scope
	approveDomain(t, port, requestID, "global")
	t.Log("Approved domain with global scope")

	// Wait for the proxy request goroutine to complete
	wg.Wait()

	if proxyErr != nil && strings.Contains(proxyOutput, "not found") {
		t.Skip("curl not available in container image")
	}

	// Verify the CONNECT was not rejected (403 would mean approval failed)
	trimmedOutput := strings.TrimSpace(proxyOutput)
	if trimmedOutput == "403" {
		t.Errorf("Expected CONNECT to be approved (not 403), got: %q", trimmedOutput)
	}

	// Verify the global approval file was created on disk.
	globalApprovalPath := config.GlobalDecisionPath()
	approvals, err := config.LoadGlobalDecisions()
	if err != nil {
		t.Fatalf("Failed to load global approvals: %v", err)
	}

	// Check that the test domain is in the approved list
	found := false
	for _, d := range approvals.Domains {
		if d == testDomain || d == testDomain+":443" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected domain %q in global approvals at %s, got domains: %v",
			testDomain, globalApprovalPath, approvals.Domains)
	}

	// Verify static global config was NOT modified
	staticConfigAfter, _ := os.ReadFile(globalConfigPath)
	if string(staticConfigBefore) != string(staticConfigAfter) {
		t.Error("Static global config was modified; expected it to remain unchanged")
	}
}

// TestDomainApprovalPersistence_SubsequentRequestAllowed verifies that after a
// domain is approved and persisted, subsequent requests to the same domain do
// not require re-approval (served from the reloaded allowlist).
func TestDomainApprovalPersistence_SubsequentRequestAllowed(t *testing.T) {
	tc := createAuthenticatedTestContainer(t, "approval-subseq")
	guardianHost := guardian.ContainerName()
	port := approvalPort(t)

	// Wait for proxy to be ready
	if err := waitForPort(t, tc.Name, guardianHost, 3128, 5*time.Second); err != nil {
		t.Logf("Warning: %v, proceeding anyway", err)
	}

	testDomain := "subsequent-test.example.com"

	// First request: will block in approval queue
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = execInContainer(t, tc.Name,
			"sh", "-c",
			"curl -v --proxy http://"+guardianHost+":3128 --proxy-user :"+tc.Token+
				" --max-time 15 https://"+testDomain+"/ 2>&1")
	}()

	// Approve the first request with project scope
	requestID := waitForPendingDomain(t, port, testDomain, 10*time.Second)
	approveDomain(t, port, requestID, "project")
	wg.Wait()

	// Give the guardian a moment to reload the allowlist after config write
	time.Sleep(500 * time.Millisecond)

	// Second request: should NOT require approval (domain is now in allowlist via reloaded approvals).
	// Use a short max-time. If the domain requires re-approval, the request would block
	// for 3s (approval timeout) and then return 403.
	// If the domain is in the allowlist, the CONNECT succeeds immediately (200),
	// then upstream connection fails (the domain doesn't resolve) but CONNECT was 200.
	startTime := time.Now()
	output, _ := execInContainer(t, tc.Name,
		"sh", "-c",
		"curl -v --proxy http://"+guardianHost+":3128 --proxy-user :"+tc.Token+
			" --max-time 8 https://"+testDomain+"/ 2>&1 | grep -oE 'HTTP/[0-9.]+ [0-9]+' | head -1 | awk '{print $2}'")
	elapsed := time.Since(startTime)

	output = strings.TrimSpace(output)

	if strings.Contains(output, "not found") {
		t.Skip("curl not available in container image")
	}

	// The domain should now be in the allowlist. The CONNECT should succeed (200)
	// and the response should come quickly (not waiting for approval timeout).
	if output == "403" {
		t.Errorf("Second request returned 403; expected domain to be in allowlist after approval persistence")
	}

	// If response took more than 2.5s, the domain was probably re-queued for approval
	// (approval timeout is 3s). A pre-approved domain should respond much faster.
	if elapsed > 2500*time.Millisecond {
		t.Errorf("Second request took %v; expected fast response for pre-approved domain (approval timeout is 3s)", elapsed)
	}
}
