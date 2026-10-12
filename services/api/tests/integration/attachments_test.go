//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/blob"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
)

type cleanFixtureScanner struct{}

func (cleanFixtureScanner) Scan(context.Context, blob.Object) (blob.Verdict, error) {
	return blob.Clean, nil
}
func (f authFixture) uploadFile(t *testing.T, client *http.Client, q quests.Quest, name string, data []byte) authResponse {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	if err := writer.WriteField("quest_id", q.ID); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("version", version(q)); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := http.NewRequest("POST", f.server.URL+"/api/v1/attachments", &buffer)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("Origin", "http://localhost:3000")
	r.Header.Set("X-CSRF-Token", f.csrf(t, client))
	response, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return authResponse{status: response.StatusCode, header: response.Header, body: body}
}
func readAttachment(t *testing.T, r authResponse) quests.Attachment {
	t.Helper()
	var body struct {
		Data quests.Attachment `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &body) != nil || body.Data.ID == "" {
		t.Fatalf("attachment %d %s", r.status, r.body)
	}
	return body.Data
}
func TestAttachmentPrivacyValidationQuotaAndDownload(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits(), cleanFixtureScanner{})
	client, _ := f.signedIn(t, "files@example.test")
	other, _ := f.signedIn(t, "files-other@example.test")
	q := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", questInput("Файлы")))
	for _, name := range []string{"fake.png", "../secret.txt", `..\secret.txt`, "file.svg"} {
		if r := f.uploadFile(t, client, q, name, []byte("<html>unsafe</html>")); r.status != 422 {
			t.Fatal("unsafe file accepted", name, r.status)
		}
	}
	a := readAttachment(t, f.uploadFile(t, client, q, "notes.txt", []byte("Private notes")))
	if a.Verdict != blob.Clean {
		t.Fatal("fixture scanner not used")
	}
	if r := f.privateRequest(t, other, "GET", "/attachments/"+a.ID+"/download", "", nil); r.status != 404 {
		t.Fatal("foreign file download")
	}
	r := f.privateRequest(t, client, "GET", "/attachments/"+a.ID+"/download", "", nil)
	if r.status != 200 || string(r.body) != "Private notes" || !bytes.Contains([]byte(r.header.Get("Content-Disposition")), []byte("attachment")) || r.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("unsafe download")
	}
	for range 4 {
		q = readQuest(t, f.privateRequest(t, client, "GET", "/quests/"+q.ID, "", nil))
		readAttachment(t, f.uploadFile(t, client, q, "extra.txt", []byte("extra")))
	}
	q = readQuest(t, f.privateRequest(t, client, "GET", "/quests/"+q.ID, "", nil))
	if r := f.uploadFile(t, client, q, "sixth.txt", []byte("extra")); r.status != 422 {
		t.Fatal("file quota ignored")
	}
	if r := f.privateRequest(t, other, "DELETE", "/attachments/"+a.ID, `"2"`, struct{}{}); r.status != 404 {
		t.Fatal("foreign deletion")
	}
	if r := f.privateRequest(t, client, "DELETE", "/attachments/"+a.ID, `"2"`, struct{}{}); r.status != 200 {
		t.Fatal("delete failed")
	}
	if r := f.privateRequest(t, client, "GET", "/attachments/"+a.ID+"/download", "", nil); r.status != 404 {
		t.Fatal("deleted file available")
	}
}
func TestAttachmentUnavailableScannerRemainsQuarantined(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "quarantine@example.test")
	q := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", questInput("Карантин")))
	a := readAttachment(t, f.uploadFile(t, client, q, "notes.txt", []byte("private pending")))
	if a.Verdict != blob.Pending {
		t.Fatal("unscanned file declared safe")
	}
	if r := f.privateRequest(t, client, "GET", "/attachments/"+a.ID+"/download", "", nil); r.status != 409 {
		t.Fatal("pending file downloaded")
	}
	r := f.privateRequest(t, client, "POST", "/attachments/"+a.ID+"/scan", `"1"`, struct{}{})
	if readAttachment(t, r).Verdict != blob.Pending {
		t.Fatal("retry failed open")
	}
}
