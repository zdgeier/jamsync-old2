gen:
	mkdir -p gen/go && protoc --proto_path=proto --go_out=gen/go --go_opt=paths=source_relative --go-grpc_out=gen/go --go-grpc_opt=paths=source_relative proto/*.proto

clean:
	rm -r build && rm -rf jamsync-build.zip

web:
	cd cmd/web/; JAM_ENV=local go run main.go

webprod:
	cd cmd/web/; JAM_ENV=prod go run main.go --useenv

buildweb:
	GOOS=linux GOARCH=arm64 go build -o build/jamweb cmd/web/main.go; cp -R cmd/web/static build; cp -R cmd/web/template build; 

server:
	JAM_ENV=local go run cmd/server/main.go

buildserver:
	go build -o build/jamserver cmd/server/main.go 

client:
	JAM_ENV=local go run cmd/client/main.go 

buildclient:
	go build -o build/jam cmd/client/main.go && cp build/jam ~/bin/jam

buildclients:
	./allclients.sh

zipself:
	mkdir -p ./build/static && zip -r build/static/jamsync-source.zip . -x .git/\*

zipbuild:
	zip -r jamsync-build.zip build/

build: clean zipself buildserver buildclients buildweb zipbuild

test:
	go test ./...
