default: build

build:
	GOOS=linux GOARCH=amd64 go build -o main main.go

package: build
	zip sswc-api-deployment.zip main