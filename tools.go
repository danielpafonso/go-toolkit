package toolkit

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const randomStringSource = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_+"

// Tools is the type used to initiate this module. Any variable of this type will have access
// to all the methods with the reciever *Tools
type Tools struct {
	MaxFileSize        int
	AllowedFileTypes   []string
	MaxJSONSize        int
	AllowUnknownFields bool
}

// RandomString returns a string of random characters of lenght n, using randomStringSource
// as the source of characters
func (t *Tools) RandomString(n int) string {
	s, r := make([]rune, n), []rune(randomStringSource)
	for i := range s {
		p, _ := rand.Prime(rand.Reader, len(r))
		x, y := p.Uint64(), uint64(len(r))
		s[i] = r[x%y]
	}
	return string(s)
}

// UploadedFile is a struct used to save information about an uploaded file
type UploadedFile struct {
	NewFileName      string
	OriginalFileName string
	FileSize         int64
}

// UploadOneFile is justa convenienve method that calls UploadFiles by expects only one file to upload
func (t *Tools) UploadOneFile(r *http.Request, uploadDir string, rename ...bool) (*UploadedFile, error) {
	renameFile := true
	if len(rename) > 0 {
		renameFile = rename[0]
	}

	files, err := t.UploadFiles(r, uploadDir, renameFile)
	if err != nil {
		return nil, err
	}
	return files[0], nil
}

// UploadFiles uploads one or more files to a specified directory, and gives the files a random string as new name
func (t *Tools) UploadFiles(r *http.Request, uploadDir string, rename ...bool) ([]*UploadedFile, error) {
	renameFile := true
	if len(rename) > 0 {
		renameFile = rename[0]
	}

	var uploadedFiles []*UploadedFile

	if t.MaxFileSize == 0 {
		t.MaxFileSize = 1024 * 1024 * 1024
	}

	err := t.CreateDirIfNotExist(uploadDir)
	if err != nil {
		return nil, err
	}

	err = r.ParseMultipartForm(int64(t.MaxFileSize))
	if err != nil {
		return nil, errors.New("the uploaded file is too big")
	}

	for _, fHeaders := range r.MultipartForm.File {
		for _, hdr := range fHeaders {
			uploadedFiles, err = func(uploadedFiles []*UploadedFile) ([]*UploadedFile, error) {
				var uploadedFile UploadedFile
				inFile, err := hdr.Open()
				if err != err {
					return nil, err
				}
				defer inFile.Close()
				buff := make([]byte, 512)
				_, err = inFile.Read(buff)
				if err != nil {
					return nil, err
				}
				allowed := false
				fileType := http.DetectContentType(buff)
				if len(t.AllowedFileTypes) > 0 {
					for _, x := range t.AllowedFileTypes {
						if strings.EqualFold(fileType, x) {
							allowed = true
						}
					}
				} else {
					allowed = true
				}
				if !allowed {
					return nil, errors.New("the uploaded file types is not permited")
				}
				_, err = inFile.Seek(0, 0)
				if err != nil {
					return nil, err
				}
				if renameFile {
					uploadedFile.NewFileName = fmt.Sprintf("%s%s", t.RandomString(25), filepath.Ext(hdr.Filename))
				} else {
					uploadedFile.NewFileName = hdr.Filename
				}

				uploadedFile.OriginalFileName = hdr.Filename
				var outFile *os.File
				defer outFile.Close()
				if outFile, err = os.Create(filepath.Join(uploadDir, uploadedFile.NewFileName)); err != nil {
					return nil, err
				} else {
					fileSize, err := io.Copy(outFile, inFile)
					if err != nil {
						return nil, err
					}
					uploadedFile.FileSize = fileSize
				}

				uploadedFiles = append(uploadedFiles, &uploadedFile)
				return uploadedFiles, nil
			}(uploadedFiles)

			if err != nil {
				return uploadedFiles, err
			}
		}
	}
	return uploadedFiles, nil
}

// CreateDirIfNotExist creates a directory, and all necessery parents it it does not exists
func (r *Tools) CreateDirIfNotExist(path string) error {
	const mode = 0755
	if _, err := os.Stat(path); os.IsNotExist(err) {
		err := os.MkdirAll(path, mode)
		if err != nil {
			return err
		}
	}
	return nil
}

// Slugify simple meathod of creating a slog from a string
func (t *Tools) Slugify(s string) (string, error) {
	if s == "" {
		return "", errors.New("empty string not permited")
	}

	var re = regexp.MustCompile(`[^a-z\d]+`)
	slug := strings.Trim(re.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(slug) == 0 {
		return "", errors.New("slug is zero length")
	}
	return slug, nil
}

// DownloadStaticFile downloads file and allows the specifications of the display name
func (t *Tools) DownloadStaticFile(w http.ResponseWriter, r *http.Request, pathname, displayName string) {
	// tells browser to download instead of displaying file
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", displayName))

	http.ServeFile(w, r, pathname)
}

// JSONResponse is the type used for sending JSON around
type JSONResponse struct {
	Error   bool        `json:"error"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ReadJson tries to read a body of a request and convents it from json into a go object
func (t *Tools) ReadJson(w http.ResponseWriter, r *http.Request, data interface{}) error {
	maxBytes := 1024 * 1024
	if t.MaxJSONSize != 0 {
		maxBytes = t.MaxJSONSize
	}

	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))
	dec := json.NewDecoder(r.Body)
	if !t.AllowUnknownFields {
		dec.DisallowUnknownFields()
	}

	// decode data
	err := dec.Decode(data)
	if err != nil {
		var syntaxError *json.SyntaxError
		var unmarshallTypeError *json.UnmarshalTypeError
		var invalidUnmarshalError *json.InvalidUnmarshalError

		switch {
		case errors.As(err, &syntaxError):
			return fmt.Errorf("body contains badly-formated JSON (at character %d)", syntaxError.Offset)

		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains badly-formated JSON")

		case errors.As(err, &unmarshallTypeError):
			if unmarshallTypeError.Field != "" {
				return fmt.Errorf("body contains incorrect JSON type for field %q", unmarshallTypeError.Field)
			}
			return fmt.Errorf("body contains incorrect JSON type (as character%d)", unmarshallTypeError.Offset)

		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")

		case strings.HasPrefix(err.Error(), "json: unknown field"):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field")
			return fmt.Errorf("body contains unknow key %s", fieldName)

		case err.Error() == "http: request body too large":
			return fmt.Errorf("body must not by larger than %d bytes", maxBytes)

		case errors.As(err, &invalidUnmarshalError):
			return fmt.Errorf("error unmarshalling JSON: %s", err.Error())
		default:
			return err
		}
	}

	// check if there is more than one json value
	err = dec.Decode(&struct{}{})
	if err != io.EOF {
		return errors.New("body must contain only one JSON value")
	}

	return nil
}

// WriteJson takes a response status code and arbitary data and writes json to the client
func (t *Tools) WriteJson(w http.ResponseWriter, status int, data interface{}, headers ...http.Header) error {
	out, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if len(headers) > 0 {
		for key, value := range headers[0] {
			w.Header()[key] = value
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err = w.Write(out)
	if err != nil {
		return err
	}
	return nil
}

// ErrorJson takes an error and status code (optionally), generate and sends a JSON error message
func (t *Tools) ErrorJson(w http.ResponseWriter, err error, status ...int) error {
	statusCode := http.StatusBadRequest
	if len(status) > 0 {
		statusCode = status[0]
	}
	var payload JSONResponse
	payload.Error = true
	payload.Message = err.Error()

	return t.WriteJson(w, statusCode, payload)
}

// PushJsontoRemote post data to some url as json and returns the response, status code and error it any.
// client parameter is optional and defaults to the standard http.Client
func (t *Tools) PushJsontoRemote(uri string, data interface{}, client ...*http.Client) (*http.Response, int, error) {
	// create json
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, 0, err
	}
	// check for http client
	httpClient := &http.Client{}
	if len(client) > 0 {
		httpClient = client[0]
	}
	// build request
	request, err := http.NewRequest("POST", uri, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	// call uri
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	// send info back
	return response, response.StatusCode, nil
}
