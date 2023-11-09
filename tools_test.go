package toolkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
)

// set up http client that send back arbitary response, which enable write tests without having a remote API
type RoundTripFunc func(req *http.Request) *http.Response

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}
func NewTestClient(fn RoundTripFunc) *http.Client {
	return &http.Client{
		Transport: fn,
	}
}

func TestTools_RandomString(t *testing.T) {
	var testTools Tools

	s := testTools.RandomString(10)
	if len(s) != 10 {
		t.Error("wrong lenght random string returned")
	}
}

var uploadTests = []struct {
	name          string
	allowedTypes  []string
	renameFile    bool
	errorExpected bool
}{
	{name: "allowed no rename", allowedTypes: []string{"image/jpeg", "image/png"}, renameFile: false, errorExpected: false},
	{name: "allowed rename", allowedTypes: []string{"image/jpeg", "image/png"}, renameFile: true, errorExpected: false},
	{name: "not allowed", allowedTypes: []string{"image/jpeg"}, renameFile: false, errorExpected: true},
}

func TestTools_UploadFiles(t *testing.T) {
	for _, e := range uploadTests {
		// set up a pip to avoid buffering
		pr, pw := io.Pipe()
		writer := multipart.NewWriter(pw)
		wg := sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer writer.Close()
			defer wg.Done()

			/// create form datat filed "file"
			part, err := writer.CreateFormFile("file", "./testdata/img.png")
			if err != nil {
				t.Error(err)
			}
			f, err := os.Open("./testdata/img.png")
			if err != nil {
				t.Error(err)
			}
			img, _, err := image.Decode(f)
			if err != nil {
				t.Error("error decoding image", err)
			}
			err = png.Encode(part, img)
			if err != nil {
				t.Error(t)
			}
		}()

		// read from the pipe which receives data
		request := httptest.NewRequest("POST", "/", pr)
		request.Header.Add("Content-Type", writer.FormDataContentType())

		var testTools Tools
		testTools.AllowedFileTypes = e.allowedTypes

		uploadedFiles, err := testTools.UploadFiles(request, "./testdata/uploads", e.renameFile)
		if err != nil && !e.errorExpected {
			t.Error(err)
		}

		if !e.errorExpected {
			if _, err := os.Stat(fmt.Sprintf("./testdata/uploads/%s", uploadedFiles[0].NewFileName)); os.IsNotExist(err) {
				t.Errorf("%s: expected file to exists: %s", e.name, err.Error())
			}

			// cleanup
			_ = os.Remove(fmt.Sprintf("./testdata/uploads/%s", uploadedFiles[0].NewFileName))

			if e.errorExpected && err == nil {
				t.Errorf("%s: error expected but none received", e.name)
			}
			wg.Wait()
		}
	}
}

func TestTools_UploadOnFile(t *testing.T) {
	// set up a pip to avoid buffering
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer writer.Close()

		/// create form datat filed "file"
		part, err := writer.CreateFormFile("file", "./testdata/img.png")
		if err != nil {
			t.Error(err)
		}
		f, err := os.Open("./testdata/img.png")
		if err != nil {
			t.Error(err)
		}
		img, _, err := image.Decode(f)
		if err != nil {
			t.Error("error decoding image", err)
		}
		err = png.Encode(part, img)
		if err != nil {
			t.Error(t)
		}
	}()

	// read from the pipe which receives data
	request := httptest.NewRequest("POST", "/", pr)
	request.Header.Add("Content-Type", writer.FormDataContentType())

	var testTools Tools

	uploadedFiles, err := testTools.UploadOneFile(request, "./testdata/uploads", true)
	if err != nil {
		t.Error(err)
	}

	if _, err := os.Stat(fmt.Sprintf("./testdata/uploads/%s", uploadedFiles.NewFileName)); os.IsNotExist(err) {
		t.Errorf("expected file to exist: %s", err.Error())
	}

	// cleanup
	_ = os.Remove(fmt.Sprintf("./testdata/uploads/%s", uploadedFiles.NewFileName))

}

func TestTools_CreateDirIfNotExsi(t *testing.T) {
	var testTool Tools

	err := testTool.CreateDirIfNotExist("./testdata/myDir")
	if err != nil {
		t.Error(err)
	}

	err = testTool.CreateDirIfNotExist("./testdata/myDir")
	if err != nil {
		t.Error(err)
	}

	_ = os.Remove("./testdata/myDir")
}

var slugTests = []struct {
	name          string
	s             string
	expected      string
	errorExpected bool
}{
	{name: "valid string", s: "Now is the time", expected: "now-is-the-time", errorExpected: false},
	{name: "valid string with digits", s: "there are 2 many people", expected: "there-are-2-many-people", errorExpected: false},
	{name: "complex string", s: "Now is the time for all GOOD men! + fish & such &^123", expected: "now-is-the-time-for-all-good-men-fish-such-123", errorExpected: false},
	{name: "empty string", s: "", expected: "", errorExpected: true},
	{name: "empty string", s: "", expected: "", errorExpected: true},
	{name: "japanese and roman", s: "Hello こんにちは、母さん mother", expected: "hello-mother", errorExpected: false},
}

func TestTools_Slugify(t *testing.T) {
	var testTool Tools

	for _, e := range slugTests {
		slug, err := testTool.Slugify(e.s)
		if err != nil && !e.errorExpected {
			t.Errorf("%s: error received when none expected: %s", e.name, err)
		}

		if !e.errorExpected && slug != e.expected {
			t.Errorf("%s: wrong slug returned; expected %s but got %s", e.name, e.expected, slug)
		}
	}
}

func TestTools_DownloadStaticFile(t *testing.T) {
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)

	var testTools Tools

	testTools.DownloadStaticFile(rr, req, "./testdata/pic.png", "face.png")
	res := rr.Result()
	defer res.Body.Close()

	if res.Header["Content-Length"][0] != "12478" {
		t.Error("wrong content length of", res.Header["Content-Length"][0])
	}

	if res.Header["Content-Disposition"][0] != "attachment; filename=\"face.png\"" {
		t.Error("wrong contect disposition")
	}

	_, err := io.ReadAll(res.Body)
	if err != nil {
		t.Error(err)
	}
}

var jsonTests = []struct {
	name          string
	json          string
	errorExpected bool
	maxSize       int
	AllowUnknown  bool
}{
	{name: "good json", json: `{"foo": "bar"}`, errorExpected: false, maxSize: 1024, AllowUnknown: false},
	{name: "badly formated json", json: `{"bar"}`, errorExpected: true, maxSize: 1024, AllowUnknown: false},
	{name: "incorrect type", json: `{"foo": 1}`, errorExpected: true, maxSize: 1024, AllowUnknown: false},
	{name: "two json files", json: `{"foo": "bar"}{"alpha": "beta"}`, errorExpected: true, maxSize: 1024, AllowUnknown: false},
	{name: "empty body", json: ``, errorExpected: true, maxSize: 1024, AllowUnknown: false},
	{name: "syntax error in json", json: `{"foo": 1"`, errorExpected: true, maxSize: 1024, AllowUnknown: false},
	{name: "unknown field in json", json: `{"fooo": "bar"}`, errorExpected: true, maxSize: 1024, AllowUnknown: false},
	{name: "allow unknow fields in json", json: `{"fooo":"bar"}`, errorExpected: false, maxSize: 1024, AllowUnknown: true},
	{name: "missing field name", json: `{jack: "bar"}`, errorExpected: true, maxSize: 1024, AllowUnknown: true},
	{name: "file too large", json: `{"foo": "bar"}`, errorExpected: true, maxSize: 5, AllowUnknown: false},
	{name: "not json", json: `Hello, world!`, errorExpected: true, maxSize: 1024, AllowUnknown: false},
}

func TestTools_ReadJSON(t *testing.T) {
	var testTools Tools

	for _, e := range jsonTests {
		// set tools configs
		testTools.MaxJSONSize = e.maxSize
		testTools.AllowUnknownFields = e.AllowUnknown

		var decodedJSON struct {
			Foo string `json:"foo"`
		}

		req, err := http.NewRequest("POST", "/", bytes.NewReader([]byte(e.json)))
		if err != nil {
			t.Log("Error", err)
		}
		rr := httptest.NewRecorder()
		err = testTools.ReadJson(rr, req, &decodedJSON)
		if e.errorExpected && err == nil {
			t.Errorf("%s: error expected, but non received", e.name)
		}

		if !e.errorExpected && err != nil {
			t.Errorf("%s: error not expected, but one received: %s", e.name, err.Error())
		}
		req.Body.Close()
	}
}

func TestTools_WriteJSON(t *testing.T) {
	var testTools Tools

	rr := httptest.NewRecorder()
	payload := JSONResponse{
		Error:   false,
		Message: "foobar",
	}
	headers := make(http.Header)
	headers.Add("FOO", "BAR")
	err := testTools.WriteJson(rr, http.StatusOK, payload, headers)
	if err != nil {
		t.Errorf("failed to write json: %v", err)
	}
}

func TestTools_ErrorJSON(t *testing.T) {
	var testTools Tools

	rr := httptest.NewRecorder()
	err := testTools.ErrorJson(rr, errors.New("some error"), http.StatusServiceUnavailable)
	if err != nil {
		t.Error(err)
	}
	var payload JSONResponse
	decoder := json.NewDecoder(rr.Body)
	err = decoder.Decode(&payload)
	if err != nil {
		t.Error("received errro when decoding JSON")
	}
	if !payload.Error {
		t.Error("error set to false in JSON, and should be True")
	}
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("wrong status code returned, expected 503 got %d", rr.Code)
	}
}

func TestTools_PushJsonToRemote(t *testing.T) {
	// custom client
	client := NewTestClient(func(req *http.Request) *http.Response {
		// test request parameters
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString("foobar")),
			Header:     make(http.Header),
		}
	})

	var testTools Tools
	var foo struct {
		Bar string `json:"bar"`
	}
	foo.Bar = "data"
	_, _, err := testTools.PushJsontoRemote("http://example.com/some/path", foo, client)
	if err != nil {
		t.Error("failed to call remote url:", err)
	}
}
