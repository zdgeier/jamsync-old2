gen:
	mkdir -p gen/go && protoc --proto_path=proto --go_out=gen/go --go_opt=paths=source_relative --go-grpc_out=gen/go --go-grpc_opt=paths=source_relative proto/*.proto

clean:
	rm -rf jamsync-build && rm -rf jamsync-build.zip

endclean:
	rm -rf jamsync-build && rm -rf jamsync-build.zip

web:
	cd cmd/web/; JAM_ENV=local go run main.go

webprod:
	cd cmd/web/; JAM_ENV=prod go run main.go --useenv

buildweb:
	GOOS=linux GOARCH=arm64 go build -o jamsync-build/jamweb cmd/web/main.go; cp -R cmd/web/static jamsync-build/; cp -R cmd/web/template jamsync-build/; 

server:
	JAM_ENV=local go run cmd/server/main.go

buildserver:
	go build -o jamsync-build/jamserver cmd/server/main.go 

client:
	JAM_ENV=local go run cmd/client/main.go 

buildclient:
	go build -o jamsync-build/jam cmd/client/main.go && cp jamsync-build/jam ~/bin/jam

buildclients:
	./allclients.sh

backup:
	mkdir -p ./jamsync-build/static && zip -r jamsync-build/static/jamsync-source.zip . -x .git/\* && cp jamsync-build/static/jamsync-source.zip ~/Documents/temp

zipself:
	mkdir -p ./jamsync-build/static && zip -r jamsync-build/static/jamsync-source.zip . -x .git/\*

zipbuild:
	zip -r jamsync-build.zip jamsync-build/

uploadbuild:
	scp -i ~/jamsynckeypair.pem ./jamsync-build.zip ec2-user@prod.jamsync.dev:~/jamsync-build.zip

ssh:
	ssh -i ~/jamsynckeypair.pem ec2-user@prod.jamsync.dev

build: clean zipself buildserver buildclients buildweb zipbuild uploadbuild endclean

test:
	go test ./...
