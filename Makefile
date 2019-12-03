default: build

build:
	GOOS=linux GOARCH=amd64 go build -o main main.go

package: build
	zip sswc-api-deployment.zip main

deploy-test: package
	aws s3 cp sswc-api-deployment.zip s3://sierra-springs-watch-test-usw2-deployment/0.1.0/sswc-api-deployment.zip

deploy-prod: package
	aws s3 cp sswc-api-deployment.zip s3://sierra-springs-watch-usw2-deployment/0.1.0/sswc-api-deployment.zip