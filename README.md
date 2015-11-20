# GoUpload

This API is a basic solution for a file server. The API is written in Go and backed by both S3 and MongoDB.
Responses are in JSON, and responds to the following endpoints:

*Routes are prefixed with `/v{version_number}*

- [GET] /files/{id} - returns the file matching the id specified
- [PUT] /files - creates a new file

# Setup
The API is currently running on an EC2 instance at http://52.23.204.111:3000:

# Response Format
Response format will be in JSON, and follow the structure below:
```json
{
    "success": true,
    "status_code": 200,
    "status_text": "OK",
    "error_code": 0,
    "error_text": "No error",
    "content": // file information (ID & URL)
}
```
# Endpoints

##### GET `/files/{id}`
Returns the file with the matching ID.
e.g. `curl http://52.23.204.111:3000/v1/files/{id}`

Returns the file with the matching ID and password.
e.g. `curl -X GET -F "password=YOURPASSWORD" http://52.23.204.111:3000/v1/files/{id}`

##### PUT `/files`
Creates a new file.
e.g. `curl -X PUT -F "file=@[file_path]" http://52.23.204.111:3000/v1/files`

Creates a new file with a password.
e.g. `curl -X PUT -F "file=@[file_path]" -F "password=YOURPASSWORD" http://52.23.204.111:3000/v1/files`

# Design
The biggest hurdle in this technical challenge was picking the right tools for the job. Based on the project requirements I knew I needed a database to store file information, a place to store files, and an application to handle responses, uploading files, and storing information on our database.

I decided to go with Go because of its performance, portability, and ability to handle concurrent requests. 

I decided to go with S3 for storage because files could be really big and we wouldn't want to store them on our machines since a handful of 200gb files could easily eat up a machine's space. S3 also seemed like a scalable solution in the sense that it removed the need of us having to worry about space and anyone could access it. I believe accessibility to these files is something that is important and having these files in a place where any application could access is huge.

I went with Mongo as my database because of its flexibility and it's ease to scale. The data I was dealing with is quite simple and did not require me to cross-reference data. As a result, I was able to store all necessary information in one document.

Since I needed to handle provided passwords, I hashed the given passwords and stored the hash in the database using bcrypt. 

# Approach
I approached this challenge by breaking down the mission components into separate phases of the product. The individual components I broke the project into were:

1. Create a handler for a PUT request.
  1. Accept a file and password.
  2. Check whether or not a file was sent in the request.
  3. Upload the file to S3 and retrieve the URL.
  4. Generate a UUID for the password if one was provided.
  5. Save the file information to Mongo.
  6. Generate the proper HTTP response.
  7. Write the response.

2. Create a handler for a GET request.
  1. Check whether the ID is valid or not.
  2. Fetch the document that matches the ID.
  3. Check whether or not the resource has been retrieved.
  4. If the resource has not been retrieved I will proceed to delete it from S3 and update the document.
  5. Generate the proper HTTP response.
  6. Write the response.
