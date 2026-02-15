package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aim-chat/go-backend/internal/identity"
	"aim-chat/go-backend/pkg/models"
)

type publishFailureTestHook interface {
	SetPublishFailuresForTesting(failures map[string]error)
}

func requirePublishFailureHook(t *testing.T, srv *Server) publishFailureTestHook {
	t.Helper()
	hook, ok := srv.service.(publishFailureTestHook)
	if !ok {
		t.Fatal("expected publish failure test hook")
	}
	return hook
}

func callRPC(t *testing.T, srv *Server, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.HandleRPC(rec, req)
	return rec
}

func decodeRPCBody(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
}

func requireRPCOK(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRPCRejectsNonLocalOrigin(t *testing.T) {
	srv := newTestServer(t)

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"identity.get","params":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	srv.HandleRPC(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRPCDeviceRevokeReturnsPartialDeliveryCode(t *testing.T) {
	srv := newTestServer(t)
	hook := requirePublishFailureHook(t, srv)
	if err := srv.service.AddContact("aim1_rpc_partial_01", "alice"); err != nil {
		t.Fatalf("add contact #1 failed: %v", err)
	}
	if err := srv.service.AddContact("aim1_rpc_partial_02", "bob"); err != nil {
		t.Fatalf("add contact #2 failed: %v", err)
	}
	if err := srv.service.StartNetworking(context.Background()); err != nil {
		t.Fatalf("start networking failed: %v", err)
	}
	defer func() { _ = srv.service.StopNetworking(context.Background()) }()

	hook.SetPublishFailuresForTesting(map[string]error{
		"aim1_rpc_partial_02": errors.New("dial failed"),
	})

	added, err := srv.service.AddDevice("work-laptop")
	if err != nil {
		t.Fatalf("add device failed: %v", err)
	}

	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      4991,
		"method":  "device.revoke",
		"params":  []string{added.ID},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.HandleRPC(rec, req)

	var resp struct {
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected rpc error for partial delivery failure")
	}
	if resp.Error.Code != -32053 {
		t.Fatalf("expected code -32053, got %d", resp.Error.Code)
	}
}

func TestRPCDeviceRevokeReturnsFullDeliveryCode(t *testing.T) {
	srv := newTestServer(t)
	hook := requirePublishFailureHook(t, srv)
	if err := srv.service.AddContact("aim1_rpc_full_01", "alice"); err != nil {
		t.Fatalf("add contact #1 failed: %v", err)
	}
	if err := srv.service.AddContact("aim1_rpc_full_02", "bob"); err != nil {
		t.Fatalf("add contact #2 failed: %v", err)
	}
	if err := srv.service.StartNetworking(context.Background()); err != nil {
		t.Fatalf("start networking failed: %v", err)
	}
	defer func() { _ = srv.service.StopNetworking(context.Background()) }()

	hook.SetPublishFailuresForTesting(map[string]error{
		"aim1_rpc_full_01": errors.New("link down"),
		"aim1_rpc_full_02": errors.New("dial failed"),
	})

	added, err := srv.service.AddDevice("work-laptop")
	if err != nil {
		t.Fatalf("add device failed: %v", err)
	}

	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      4992,
		"method":  "device.revoke",
		"params":  []string{added.ID},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.HandleRPC(rec, req)

	var resp struct {
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected rpc error for full delivery failure")
	}
	if resp.Error.Code != -32054 {
		t.Fatalf("expected code -32054, got %d", resp.Error.Code)
	}
}

func TestRPCRequiresTokenWhenConfigured(t *testing.T) {
	t.Setenv("AIM_RPC_TOKEN", "test-token")
	srv := newTestServer(t)

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"identity.get","params":[]}`)

	reqNoToken := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	recNoToken := httptest.NewRecorder()
	srv.HandleRPC(recNoToken, reqNoToken)
	if recNoToken.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", recNoToken.Code)
	}

	reqWithToken := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	reqWithToken.Header.Set("X-AIM-RPC-Token", "test-token")
	recWithToken := httptest.NewRecorder()
	srv.HandleRPC(recWithToken, reqWithToken)
	if recWithToken.Code != http.StatusOK {
		t.Fatalf("expected 200 with token, got %d", recWithToken.Code)
	}

	reqWithQueryToken := httptest.NewRequest(http.MethodPost, "/rpc?rpc_token=test-token", bytes.NewReader(body))
	recWithQueryToken := httptest.NewRecorder()
	srv.HandleRPC(recWithQueryToken, reqWithQueryToken)
	if recWithQueryToken.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with query token, got %d", recWithQueryToken.Code)
	}
}

func TestRPCRejectsOversizedBody(t *testing.T) {
	srv := newTestServer(t)

	large := `{"jsonrpc":"2.0","id":1,"method":"identity.get","params":["` + strings.Repeat("x", int(maxRPCBodyBytes)) + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(large))
	rec := httptest.NewRecorder()
	srv.HandleRPC(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}
}

func TestRPCIdentityGet(t *testing.T) {
	srv := newTestServer(t)

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"identity.get","params":[]}`)
	rec := callRPC(t, srv, body)
	requireRPCOK(t, rec)

	var resp struct {
		Result models.Identity `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	decodeRPCBody(t, rec, &resp)
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", *resp.Error)
	}
	if !strings.HasPrefix(resp.Result.ID, "aim1") {
		t.Fatalf("expected aim1 prefix, got %s", resp.Result.ID)
	}
}

func TestRPCIdentitySelfContactCard(t *testing.T) {
	srv := newTestServer(t)

	body := []byte(`{"jsonrpc":"2.0","id":50,"method":"identity.self_contact_card","params":["local-user"]}`)
	rec := callRPC(t, srv, body)
	requireRPCOK(t, rec)

	var resp struct {
		Result models.ContactCard `json:"result"`
		Error  *rpcError          `json:"error"`
	}
	decodeRPCBody(t, rec, &resp)
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", *resp.Error)
	}
	if resp.Result.IdentityID == "" || len(resp.Result.PublicKey) == 0 || len(resp.Result.Signature) == 0 {
		t.Fatal("self contact card must include identity id, public key, and signature")
	}
}

func TestRPCNetworkStatus(t *testing.T) {
	srv := newTestServer(t)

	body := []byte(`{"jsonrpc":"2.0","id":101,"method":"network.status","params":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.HandleRPC(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Result struct {
			Status    string `json:"status"`
			PeerCount int    `json:"peer_count"`
		} `json:"result"`
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", *resp.Error)
	}
	if resp.Result.Status == "" {
		t.Fatal("network status should not be empty")
	}
}

func TestRPCMetricsGet(t *testing.T) {
	srv := newTestServer(t)

	body := []byte(`{"jsonrpc":"2.0","id":102,"method":"metrics.get","params":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.HandleRPC(rec, req)

	var resp struct {
		Result models.MetricsSnapshot `json:"result"`
		Error  *rpcError              `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", *resp.Error)
	}
	if resp.Result.ErrorCounters == nil {
		t.Fatal("metrics must include error_counters")
	}
	if resp.Result.OperationStats == nil {
		t.Fatal("metrics must include operation_stats")
	}
}

func TestRPCContactVerifyAndAdd(t *testing.T) {
	srv := newTestServer(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	id, err := identity.BuildIdentityID(pub)
	if err != nil {
		t.Fatalf("build id failed: %v", err)
	}
	card, err := identity.SignContactCard(id, "bob", pub, priv)
	if err != nil {
		t.Fatalf("sign card failed: %v", err)
	}

	verifyPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "contact.verify",
		"params":  []models.ContactCard{card},
	}
	verifyBody, _ := json.Marshal(verifyPayload)
	verifyReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(verifyBody))
	verifyRec := httptest.NewRecorder()
	srv.HandleRPC(verifyRec, verifyReq)

	var verifyResp struct {
		Result map[string]bool `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal(verifyRec.Body.Bytes(), &verifyResp); err != nil {
		t.Fatalf("decode verify response failed: %v", err)
	}
	if verifyResp.Error != nil || !verifyResp.Result["valid"] {
		t.Fatal("contact.verify should return valid=true")
	}

	addPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "contact.add",
		"params":  []models.ContactCard{card},
	}
	addBody, _ := json.Marshal(addPayload)
	addReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(addBody))
	addRec := httptest.NewRecorder()
	srv.HandleRPC(addRec, addReq)

	var addResp struct {
		Result map[string]bool `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal(addRec.Body.Bytes(), &addResp); err != nil {
		t.Fatalf("decode add response failed: %v", err)
	}
	if addResp.Error != nil || !addResp.Result["added"] {
		t.Fatal("contact.add should return added=true")
	}

	listPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "contact.list",
		"params":  []any{},
	}
	listBody, _ := json.Marshal(listPayload)
	listRec := callRPC(t, srv, listBody)

	var listResp struct {
		Result []models.Contact `json:"result"`
		Error  *rpcError        `json:"error"`
	}
	decodeRPCBody(t, listRec, &listResp)
	if listResp.Error != nil {
		t.Fatalf("unexpected contact.list error: %+v", *listResp.Error)
	}
	if len(listResp.Result) != 1 || listResp.Result[0].ID != card.IdentityID {
		t.Fatal("contact.list should include added contact")
	}
}

func TestRPCContactAddByIdentityIDViaContactAdd(t *testing.T) {
	srv := newTestServer(t)

	addPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      31,
		"method":  "contact.add",
		"params":  []string{"aim1UUMgCUXE93BxtwVDUivN2q3eYPKwaPkqjnNp9QVV9pF", "Bob"},
	}
	addBody, _ := json.Marshal(addPayload)
	addReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(addBody))
	addRec := httptest.NewRecorder()
	srv.HandleRPC(addRec, addReq)

	var addResp struct {
		Result map[string]bool `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal(addRec.Body.Bytes(), &addResp); err != nil {
		t.Fatalf("decode add response failed: %v", err)
	}
	if addResp.Error != nil || !addResp.Result["added"] {
		t.Fatal("contact.add with identity id should return added=true")
	}

	listPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      32,
		"method":  "contact.list",
		"params":  []any{},
	}
	listBody, _ := json.Marshal(listPayload)
	listRec := callRPC(t, srv, listBody)

	var listResp struct {
		Result []models.Contact `json:"result"`
		Error  *rpcError        `json:"error"`
	}
	decodeRPCBody(t, listRec, &listResp)
	if listResp.Error != nil {
		t.Fatalf("unexpected contact.list error: %+v", *listResp.Error)
	}
	if len(listResp.Result) != 1 || listResp.Result[0].ID != "aim1UUMgCUXE93BxtwVDUivN2q3eYPKwaPkqjnNp9QVV9pF" {
		t.Fatal("contact.list should include identity-id added contact")
	}
}

func TestRPCContactRemove(t *testing.T) {
	srv := newTestServer(t)
	const contactID = "aim1UUMgCUXE93BxtwVDUivN2q3eYPKwaPkqjnNp9QVV9pF"

	addPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      41,
		"method":  "contact.add",
		"params":  []string{contactID, "Bob"},
	}
	addBody, _ := json.Marshal(addPayload)
	addReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(addBody))
	addRec := httptest.NewRecorder()
	srv.HandleRPC(addRec, addReq)

	removePayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      42,
		"method":  "contact.remove",
		"params":  []string{contactID},
	}
	removeBody, _ := json.Marshal(removePayload)
	removeReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(removeBody))
	removeRec := httptest.NewRecorder()
	srv.HandleRPC(removeRec, removeReq)

	var removeResp struct {
		Result map[string]bool `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal(removeRec.Body.Bytes(), &removeResp); err != nil {
		t.Fatalf("decode remove response failed: %v", err)
	}
	if removeResp.Error != nil || !removeResp.Result["removed"] {
		t.Fatal("contact.remove should return removed=true")
	}
}

func TestRPCFilePut(t *testing.T) {
	srv := newTestServer(t)
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      51,
		"method":  "file.put",
		"params":  []string{"note.txt", "text/plain", base64.StdEncoding.EncodeToString([]byte("hello"))},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.HandleRPC(rec, req)

	var resp struct {
		Result models.AttachmentMeta `json:"result"`
		Error  *rpcError             `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected file.put error: %+v", *resp.Error)
	}
	if resp.Result.ID == "" || resp.Result.Size != 5 {
		t.Fatal("file.put must return attachment metadata")
	}
}

func TestRPCIdentitySeedLifecycle(t *testing.T) {
	srv := newTestServer(t)

	createBody := []byte(`{"jsonrpc":"2.0","id":10,"method":"identity.create","params":["pass-123"]}`)
	createReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(createBody))
	createRec := httptest.NewRecorder()
	srv.HandleRPC(createRec, createReq)

	var createResp struct {
		Result struct {
			Identity models.Identity `json:"identity"`
			Mnemonic string          `json:"mnemonic"`
		} `json:"result"`
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}
	if createResp.Error != nil || createResp.Result.Mnemonic == "" || createResp.Result.Identity.ID == "" {
		t.Fatal("identity.create should return identity and mnemonic")
	}

	validatePayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      11,
		"method":  "identity.validate_mnemonic",
		"params":  []string{createResp.Result.Mnemonic},
	}
	validateBytes, _ := json.Marshal(validatePayload)
	validateReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(validateBytes))
	validateRec := httptest.NewRecorder()
	srv.HandleRPC(validateRec, validateReq)

	var validateResp struct {
		Result map[string]bool `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal(validateRec.Body.Bytes(), &validateResp); err != nil {
		t.Fatalf("decode validate response failed: %v", err)
	}
	if validateResp.Error != nil || !validateResp.Result["valid"] {
		t.Fatal("identity.validate_mnemonic should return valid=true")
	}

	exportPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      12,
		"method":  "identity.export_seed",
		"params":  []string{"pass-123"},
	}
	exportBytes, _ := json.Marshal(exportPayload)
	exportReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(exportBytes))
	exportRec := httptest.NewRecorder()
	srv.HandleRPC(exportRec, exportReq)

	var exportResp struct {
		Result map[string]string `json:"result"`
		Error  *rpcError         `json:"error"`
	}
	if err := json.Unmarshal(exportRec.Body.Bytes(), &exportResp); err != nil {
		t.Fatalf("decode export response failed: %v", err)
	}
	if exportResp.Error != nil || exportResp.Result["mnemonic"] != createResp.Result.Mnemonic {
		t.Fatal("identity.export_seed should return created mnemonic")
	}

	importPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      13,
		"method":  "identity.import_seed",
		"params":  []string{createResp.Result.Mnemonic, "new-pass"},
	}
	importBytes, _ := json.Marshal(importPayload)
	importReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(importBytes))
	importRec := httptest.NewRecorder()
	srv.HandleRPC(importRec, importReq)

	var importResp struct {
		Result struct {
			Identity models.Identity `json:"identity"`
		} `json:"result"`
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(importRec.Body.Bytes(), &importResp); err != nil {
		t.Fatalf("decode import response failed: %v", err)
	}
	if importResp.Error != nil || importResp.Result.Identity.ID != createResp.Result.Identity.ID {
		t.Fatal("identity.import_seed should reproduce same identity id")
	}

	changePayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      14,
		"method":  "identity.change_password",
		"params":  []string{"new-pass", "rotated-pass"},
	}
	changeBytes, _ := json.Marshal(changePayload)
	changeReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(changeBytes))
	changeRec := httptest.NewRecorder()
	srv.HandleRPC(changeRec, changeReq)

	var changeResp struct {
		Result map[string]bool `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal(changeRec.Body.Bytes(), &changeResp); err != nil {
		t.Fatalf("decode change password response failed: %v", err)
	}
	if changeResp.Error != nil || !changeResp.Result["changed"] {
		t.Fatal("identity.change_password should return changed=true")
	}

	exportAfterChangePayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      15,
		"method":  "identity.export_seed",
		"params":  []string{"rotated-pass"},
	}
	exportAfterChangeBytes, _ := json.Marshal(exportAfterChangePayload)
	exportAfterChangeReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(exportAfterChangeBytes))
	exportAfterChangeRec := httptest.NewRecorder()
	srv.HandleRPC(exportAfterChangeRec, exportAfterChangeReq)

	var exportAfterChangeResp struct {
		Result map[string]string `json:"result"`
		Error  *rpcError         `json:"error"`
	}
	if err := json.Unmarshal(exportAfterChangeRec.Body.Bytes(), &exportAfterChangeResp); err != nil {
		t.Fatalf("decode export after change response failed: %v", err)
	}
	if exportAfterChangeResp.Error != nil || exportAfterChangeResp.Result["mnemonic"] != createResp.Result.Mnemonic {
		t.Fatal("identity.export_seed with rotated password should succeed")
	}

	backupPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      16,
		"method":  "backup.export",
		"params":  []string{"I_UNDERSTAND_BACKUP_RISK", "backup-pass"},
	}
	backupBytes, _ := json.Marshal(backupPayload)
	backupReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(backupBytes))
	backupRec := httptest.NewRecorder()
	srv.HandleRPC(backupRec, backupReq)

	var backupResp struct {
		Result map[string]string `json:"result"`
		Error  *rpcError         `json:"error"`
	}
	if err := json.Unmarshal(backupRec.Body.Bytes(), &backupResp); err != nil {
		t.Fatalf("decode backup response failed: %v", err)
	}
	if backupResp.Error != nil || backupResp.Result["backup_blob"] == "" {
		t.Fatal("backup.export should return backup_blob")
	}

	backupBadPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      17,
		"method":  "backup.export",
		"params":  []string{"NO", "backup-pass"},
	}
	backupBadBytes, _ := json.Marshal(backupBadPayload)
	backupBadReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(backupBadBytes))
	backupBadRec := httptest.NewRecorder()
	srv.HandleRPC(backupBadRec, backupBadReq)

	var backupBadResp struct {
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(backupBadRec.Body.Bytes(), &backupBadResp); err != nil {
		t.Fatalf("decode backup bad response failed: %v", err)
	}
	if backupBadResp.Error == nil || backupBadResp.Error.Code != -32024 {
		t.Fatal("backup.export without consent should fail with -32024")
	}
}

func TestRPCSessionInit(t *testing.T) {
	srv := newTestServer(t)

	// Prepare verified contact first.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	contactID, err := identity.BuildIdentityID(pub)
	if err != nil {
		t.Fatalf("build id failed: %v", err)
	}
	card, err := identity.SignContactCard(contactID, "carol", pub, priv)
	if err != nil {
		t.Fatalf("sign card failed: %v", err)
	}
	addPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      200,
		"method":  "contact.add",
		"params":  []models.ContactCard{card},
	}
	addBody, _ := json.Marshal(addPayload)
	addReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(addBody))
	addRec := httptest.NewRecorder()
	srv.HandleRPC(addRec, addReq)

	peerKey := make([]byte, 32)
	for i := range peerKey {
		peerKey[i] = byte(32 - i)
	}
	initPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      201,
		"method":  "session.init",
		"params":  []string{contactID, base64.StdEncoding.EncodeToString(peerKey)},
	}
	initBody, _ := json.Marshal(initPayload)
	initReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(initBody))
	initRec := httptest.NewRecorder()
	srv.HandleRPC(initRec, initReq)

	var initResp struct {
		Result models.SessionState `json:"result"`
		Error  *rpcError           `json:"error"`
	}
	if err := json.Unmarshal(initRec.Body.Bytes(), &initResp); err != nil {
		t.Fatalf("decode init response failed: %v", err)
	}
	if initResp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", *initResp.Error)
	}
	if initResp.Result.SessionID == "" {
		t.Fatal("session.init must return session_id")
	}

	invalidPayload := []byte(`{"jsonrpc":"2.0","id":202,"method":"session.init","params":["` + contactID + `","bad-base64"]}`)
	invalidReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(invalidPayload))
	invalidRec := httptest.NewRecorder()
	srv.HandleRPC(invalidRec, invalidReq)
	var invalidResp struct {
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(invalidRec.Body.Bytes(), &invalidResp); err != nil {
		t.Fatalf("decode invalid response failed: %v", err)
	}
	if invalidResp.Error == nil || invalidResp.Error.Code != -32602 {
		t.Fatal("session.init invalid params must return -32602")
	}
}

func TestRPCMessageSendAndList(t *testing.T) {
	srv := newTestServer(t)

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	contactID, err := identity.BuildIdentityID(pub)
	if err != nil {
		t.Fatalf("build id failed: %v", err)
	}
	card, err := identity.SignContactCard(contactID, "dave", pub, priv)
	if err != nil {
		t.Fatalf("sign card failed: %v", err)
	}
	addPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      300,
		"method":  "contact.add",
		"params":  []models.ContactCard{card},
	}
	addBody, _ := json.Marshal(addPayload)
	addReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(addBody))
	addRec := httptest.NewRecorder()
	srv.HandleRPC(addRec, addReq)

	sendPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      301,
		"method":  "message.send",
		"params":  []string{contactID, "hello"},
	}
	sendBody, _ := json.Marshal(sendPayload)
	sendReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(sendBody))
	sendRec := httptest.NewRecorder()
	srv.HandleRPC(sendRec, sendReq)

	var sendResp struct {
		Result map[string]string `json:"result"`
		Error  *rpcError         `json:"error"`
	}
	if err := json.Unmarshal(sendRec.Body.Bytes(), &sendResp); err != nil {
		t.Fatalf("decode send response failed: %v", err)
	}
	if sendResp.Error != nil || sendResp.Result["message_id"] == "" {
		t.Fatal("message.send should return message_id")
	}

	listPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      302,
		"method":  "message.list",
		"params":  []any{contactID, 10, 0},
	}
	listBody, _ := json.Marshal(listPayload)
	listReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(listBody))
	listRec := httptest.NewRecorder()
	srv.HandleRPC(listRec, listReq)

	var listResp struct {
		Result []models.Message `json:"result"`
		Error  *rpcError        `json:"error"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response failed: %v", err)
	}
	if listResp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", *listResp.Error)
	}
	if len(listResp.Result) != 1 || string(listResp.Result[0].Content) != "hello" {
		t.Fatal("message.list should return sent message")
	}
}

func TestRPCMessageListRejectsInvalidPaginationParams(t *testing.T) {
	srv := newTestServer(t)
	contactID := "aim1testcontactid"

	testCases := []struct {
		name   string
		params any
	}{
		{name: "fractional_limit", params: []any{contactID, 10.5, 0}},
		{name: "fractional_offset", params: []any{contactID, 10, 0.25}},
		{name: "negative_limit", params: []any{contactID, -1, 0}},
		{name: "negative_offset", params: []any{contactID, 10, -1}},
		{name: "limit_too_large", params: []any{contactID, 1001, 0}},
		{name: "offset_too_large", params: []any{contactID, 10, 1000001}},
		{name: "overflow_limit", params: []any{contactID, 1e20, 0}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{
				"jsonrpc": "2.0",
				"id":      399,
				"method":  "message.list",
				"params":  tc.params,
			}
			body, _ := json.Marshal(payload)
			req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			srv.HandleRPC(rec, req)

			var resp struct {
				Error *rpcError `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response failed: %v", err)
			}
			if resp.Error == nil {
				t.Fatal("expected invalid params rpc error")
			}
			if resp.Error.Code != -32602 {
				t.Fatalf("expected -32602, got %d", resp.Error.Code)
			}
		})
	}
}

func TestRPCMessageStatus(t *testing.T) {
	srv := newTestServer(t)

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	contactID, err := identity.BuildIdentityID(pub)
	if err != nil {
		t.Fatalf("build id failed: %v", err)
	}
	card, err := identity.SignContactCard(contactID, "erin", pub, priv)
	if err != nil {
		t.Fatalf("sign card failed: %v", err)
	}
	addPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      350,
		"method":  "contact.add",
		"params":  []models.ContactCard{card},
	}
	addBody, _ := json.Marshal(addPayload)
	addReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(addBody))
	addRec := httptest.NewRecorder()
	srv.HandleRPC(addRec, addReq)

	sendPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      351,
		"method":  "message.send",
		"params":  []string{contactID, "hello"},
	}
	sendBody, _ := json.Marshal(sendPayload)
	sendReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(sendBody))
	sendRec := httptest.NewRecorder()
	srv.HandleRPC(sendRec, sendReq)
	var sendResp struct {
		Result map[string]string `json:"result"`
		Error  *rpcError         `json:"error"`
	}
	if err := json.Unmarshal(sendRec.Body.Bytes(), &sendResp); err != nil {
		t.Fatalf("decode send response failed: %v", err)
	}
	if sendResp.Error != nil || sendResp.Result["message_id"] == "" {
		t.Fatal("message.send should return message id")
	}

	statusPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      352,
		"method":  "message.status",
		"params":  []string{sendResp.Result["message_id"]},
	}
	statusBody, _ := json.Marshal(statusPayload)
	statusReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(statusBody))
	statusRec := httptest.NewRecorder()
	srv.HandleRPC(statusRec, statusReq)

	var statusResp struct {
		Result models.MessageStatus `json:"result"`
		Error  *rpcError            `json:"error"`
	}
	if err := json.Unmarshal(statusRec.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("decode status response failed: %v", err)
	}
	if statusResp.Error != nil {
		t.Fatalf("unexpected status error: %+v", *statusResp.Error)
	}
	if statusResp.Result.MessageID == "" || statusResp.Result.Status == "" {
		t.Fatal("message.status must return message_id and status")
	}
}

func TestRPCMessageEdit(t *testing.T) {
	srv := newTestServer(t)

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	contactID, err := identity.BuildIdentityID(pub)
	if err != nil {
		t.Fatalf("build id failed: %v", err)
	}
	card, err := identity.SignContactCard(contactID, "frank", pub, priv)
	if err != nil {
		t.Fatalf("sign card failed: %v", err)
	}
	addPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      360,
		"method":  "contact.add",
		"params":  []models.ContactCard{card},
	}
	addBody, _ := json.Marshal(addPayload)
	addReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(addBody))
	addRec := httptest.NewRecorder()
	srv.HandleRPC(addRec, addReq)

	sendPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      361,
		"method":  "message.send",
		"params":  []string{contactID, "draft"},
	}
	sendBody, _ := json.Marshal(sendPayload)
	sendReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(sendBody))
	sendRec := httptest.NewRecorder()
	srv.HandleRPC(sendRec, sendReq)
	var sendResp struct {
		Result map[string]string `json:"result"`
		Error  *rpcError         `json:"error"`
	}
	if err := json.Unmarshal(sendRec.Body.Bytes(), &sendResp); err != nil {
		t.Fatalf("decode send response failed: %v", err)
	}
	if sendResp.Error != nil {
		t.Fatalf("unexpected send error: %+v", *sendResp.Error)
	}

	editPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      362,
		"method":  "message.edit",
		"params":  []string{contactID, sendResp.Result["message_id"], "updated"},
	}
	editBody, _ := json.Marshal(editPayload)
	editReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(editBody))
	editRec := httptest.NewRecorder()
	srv.HandleRPC(editRec, editReq)

	var editResp struct {
		Result models.Message `json:"result"`
		Error  *rpcError      `json:"error"`
	}
	if err := json.Unmarshal(editRec.Body.Bytes(), &editResp); err != nil {
		t.Fatalf("decode edit response failed: %v", err)
	}
	if editResp.Error != nil {
		t.Fatalf("unexpected edit error: %+v", *editResp.Error)
	}
	if string(editResp.Result.Content) != "updated" || !editResp.Result.Edited {
		t.Fatal("message.edit must update content and set edited=true")
	}
}

func TestRPCDeviceListAddRevoke(t *testing.T) {
	srv := newTestServer(t)

	listBody := []byte(`{"jsonrpc":"2.0","id":401,"method":"device.list","params":[]}`)
	listReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(listBody))
	listRec := httptest.NewRecorder()
	srv.HandleRPC(listRec, listReq)

	var listResp struct {
		Result []models.Device `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response failed: %v", err)
	}
	if listResp.Error != nil {
		t.Fatalf("unexpected list error: %+v", *listResp.Error)
	}
	if len(listResp.Result) < 1 {
		t.Fatal("device.list should return at least primary device")
	}

	addPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      402,
		"method":  "device.add",
		"params":  []string{"work-laptop"},
	}
	addBody, _ := json.Marshal(addPayload)
	addReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(addBody))
	addRec := httptest.NewRecorder()
	srv.HandleRPC(addRec, addReq)

	var addResp struct {
		Result models.Device `json:"result"`
		Error  *rpcError     `json:"error"`
	}
	if err := json.Unmarshal(addRec.Body.Bytes(), &addResp); err != nil {
		t.Fatalf("decode add response failed: %v", err)
	}
	if addResp.Error != nil {
		t.Fatalf("unexpected add error: %+v", *addResp.Error)
	}
	if addResp.Result.ID == "" {
		t.Fatal("device.add must return device id")
	}

	revokePayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      403,
		"method":  "device.revoke",
		"params":  []string{addResp.Result.ID},
	}
	revokeBody, _ := json.Marshal(revokePayload)
	revokeReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(revokeBody))
	revokeRec := httptest.NewRecorder()
	srv.HandleRPC(revokeRec, revokeReq)

	var revokeResp struct {
		Result models.DeviceRevocation `json:"result"`
		Error  *rpcError               `json:"error"`
	}
	if err := json.Unmarshal(revokeRec.Body.Bytes(), &revokeResp); err != nil {
		t.Fatalf("decode revoke response failed: %v", err)
	}
	if revokeResp.Error != nil {
		t.Fatalf("unexpected revoke error: %+v", *revokeResp.Error)
	}
	if revokeResp.Result.DeviceID != addResp.Result.ID {
		t.Fatal("device.revoke returned wrong device id")
	}

	listAgainReq := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(listBody))
	listAgainRec := httptest.NewRecorder()
	srv.HandleRPC(listAgainRec, listAgainReq)

	var listAgainResp struct {
		Result []models.Device `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal(listAgainRec.Body.Bytes(), &listAgainResp); err != nil {
		t.Fatalf("decode second list response failed: %v", err)
	}
	if listAgainResp.Error != nil {
		t.Fatalf("unexpected second list error: %+v", *listAgainResp.Error)
	}
	revoked := false
	for _, d := range listAgainResp.Result {
		if d.ID == addResp.Result.ID {
			revoked = d.IsRevoked
		}
	}
	if !revoked {
		t.Fatal("revoked device must be marked in device.list")
	}
}
