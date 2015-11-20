package main

import (
  "encoding/json"
  "fmt"
  "io/ioutil"
  "log"
  "net/http"
  "os"
  "strings"
  "time"

  "github.com/tmilewski/goenv"
  "github.com/satori/go.uuid"
  "github.com/gorilla/mux"
  "github.com/mitchellh/goamz/aws"
  "github.com/mitchellh/goamz/s3"
  "golang.org/x/crypto/bcrypt"
  "gopkg.in/mgo.v2"
  "gopkg.in/mgo.v2/bson"
)

// Name of the Mongo Database & Collection. 
var DATABASE = "ghost-protocol"
var COLLECTION = "files"

type File struct {
  ID                bson.ObjectId `bson:"_id,omitempty"`
  Password          []byte        `json:"-"`
  PasswordProtected bool          `json:"-"`
  Accessed          bool          `json:"-"`
  URL               string        `json:"file_url"`
}

type Response struct {
  Success    bool        `json:"success"`
  StatusCode int         `json:"status_code"`
  StatusText string      `json:"status_text"`
  ErrorCode  int         `json:"error_code"`
  ErrorText  string      `json:"error_text"`
  Content    interface{} `json:"content"`
}

// Loading the required environment variables for S3.
func init() {
  err := goenv.Load()
  if err != nil {
    log.Fatal("The enviroment variable file (.env) is missing.")
    os.Exit(1)
  }
}

func main() {
  router := mux.NewRouter().StrictSlash(true)
  router.HandleFunc("/v1/files/{id}", GetFile).Methods("GET")
  router.HandleFunc("/v1/files", UploadFile).Methods("PUT")
  log.Fatal(http.ListenAndServe(":3000", router))
}

// Handlers
func UploadFile(w http.ResponseWriter, req *http.Request) {
  session := InitializeMongoSession()
  defer session.Close()
  collection := session.DB(DATABASE).C(COLLECTION)

  // Confirming whether or not the request includes a file.
  _, _, err := req.FormFile("file")
  if err != nil {
    response := GenerateResponse(http.StatusBadRequest, http.StatusText(http.StatusBadRequest), false, 0, "Invalid Form. (Missing file)")
    WriteResponse(response, w)
    return
  }

  file := CreateFile(req)

  err = collection.Insert(file)
  ErrorHandler(err)

  response := GenerateResponse(http.StatusCreated, http.StatusText(http.StatusCreated), true, 0, "No Error")
  response.Content = file
  WriteResponse(response, w)
}

func GetFile(w http.ResponseWriter, req *http.Request) {
  session := InitializeMongoSession()
  defer session.Close()
  collection := session.DB(DATABASE).C(COLLECTION)

  vars := mux.Vars(req)
  submittedFileId := string(vars["id"])
  response := &Response{}

  // Confirm whether or not the submitted id is valid.
  if bson.IsObjectIdHex(submittedFileId) == false {
    response = GenerateResponse(http.StatusBadRequest, http.StatusText(http.StatusBadRequest), false, 0, "Invalid ID format.")
    WriteResponse(response, w)
    return
  }

  file := &File{}
  fileId := bson.ObjectIdHex(submittedFileId)
  err := collection.FindId(fileId).One(file)

  // Confirm whether a file with the given id exists.
  if err != nil {
    response = GenerateResponse(http.StatusNotFound, http.StatusText(http.StatusNotFound), true, 0, "No Error.")
    WriteResponse(response, w)
    return
  }

  passwordIsCorrect := false

  if file.PasswordProtected == true {
    submittedPassword := []byte(req.FormValue("password"))
    passwordIsCorrect = IsPasswordCorrect(file.Password, submittedPassword)
  }

  // Check whether or not the correct password was given.
  if (file.PasswordProtected && passwordIsCorrect) || (file.PasswordProtected == false) {
    // Check whether or not the file has already been accessed.
    if file.Accessed == true {
      response = GenerateResponse(http.StatusGone, http.StatusText(http.StatusGone), true, 0, "No Error")
    } else {
      response = GenerateResponse(http.StatusOK, http.StatusText(http.StatusOK), true, 0, "No Error.")
      response.Content = file
      file.Accessed = true
      DeleteFileFromS3(file.URL)
      err = collection.UpdateId(fileId, file)
      ErrorHandler(err)
    }
  } else {
    response = GenerateResponse(http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized), false, 0, "")

    // Check whether there was no password provided or the password was incorrect. 
    if len(req.FormValue("password")) == 0 {
      response.ErrorText = "This file requires a password in order to be accessed. Please enter the correct password in order to access this file."
    } else {
      response.ErrorText = "Incorrect password. Please try again."
    }
  }

  WriteResponse(response, w)
  return
}

// S3 Utility Functions.
func UploadFileToS3(req *http.Request) (fileAbsoluteUrl string) {
  bucket := GetS3Bucket()
  req.ParseMultipartForm(16 << 20)

  file, header, err := req.FormFile("file")

  content, err := ioutil.ReadAll(file)
  ErrorHandler(err)

  // Creating the S3 upload path based on: today's date, uuid + filename.
  now := time.Now().Format("2006-01-02")
  uuid := uuid.NewV4()
  path := fmt.Sprintf("%v/%s-%v", now, uuid, header.Filename)

  err = bucket.Put(path, content, req.Header.Get("Content-Type"), s3.PublicRead)
  ErrorHandler(err)

  fileAbsoluteUrl = bucket.URL(path)

  return
}

func DeleteFileFromS3(fileAbsoluteUrl string) {
  bucket := GetS3Bucket()
  // Stripping the file URL, in order to just get the path relative to the S3 bucket. 
  fileRelativeUrl := strings.Replace(fileAbsoluteUrl, os.Getenv("AWS_BUCKET_ROOT_PATH"), "", -1)
  err := bucket.Del(fileRelativeUrl)
  ErrorHandler(err)
}

func GetS3Bucket() (bucket *s3.Bucket) {
  auth, err := aws.EnvAuth()
  ErrorHandler(err)

  client := s3.New(auth, aws.USEast)
  bucket = client.Bucket(os.Getenv("AWS_STORAGE_BUCKET_NAME"))
  return
}

// Password Utility Functions.
func CreatePasswordHash(rawPassword string) (bcryptHashedPassword []byte) {
  password := []byte(rawPassword)
  bcryptHashedPassword, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
  ErrorHandler(err)
  return
}

func IsPasswordCorrect(filePassword []byte, submittedPassword[]byte) bool {
  err := bcrypt.CompareHashAndPassword(filePassword, submittedPassword)

  if err != nil {
    return false
  }

  return true
}

// Mongo Utility Functions.
func InitializeMongoSession() (session *mgo.Session) {
  session, err := mgo.Dial("127.0.0.1")
  ErrorHandler(err)
  return
}

// Miscellaneous Utility Functions.
func CreateFile(req *http.Request) *File {
  file := &File{}
  file.ID = bson.NewObjectId()
  submittedPassword := req.FormValue("password")

  if len(submittedPassword) > 0 {
    password := CreatePasswordHash(submittedPassword)

    file.Password = password
    file.PasswordProtected = true
  }

  fileAbsoluteUrl := UploadFileToS3(req)
  file.URL = fileAbsoluteUrl

  return file
}

func ErrorHandler(err error) {
  if err != nil {
    panic(err)
  }
}

func GenerateResponse(statusCode int, statusText string, success bool, errorCode int, errorText string) *Response {
  response := &Response{}
  response.StatusCode = statusCode
  response.StatusText = statusText
  response.Success = success
  response.ErrorCode = errorCode
  response.ErrorText = errorText
  return response
}

func WriteResponse(response *Response, w http.ResponseWriter) {
  res, err := json.MarshalIndent(response, "", "  ")
  ErrorHandler(err)

  w.Header().Set("Content-Type", "application/json")
  w.Write(res)
}
