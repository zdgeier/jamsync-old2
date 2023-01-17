echo "Starting build process"

ARRAY=( "linux:amd64"
        "linux:arm64"
        "darwin:amd64"
        "darwin:arm64" )

for kv in "${ARRAY[@]}" ; do
    KEY=${kv%%:*}
    VALUE=${kv#*:}
    env GOOS=$KEY GOARCH=$VALUE go build -o jam -ldflags "-s -w" cmd/client/main.go 

    if [[ "$KEY" == "darwin" ]]
    then
        codesign -s $CODESIGN_IDENTITY -o runtime -v jam
    fi

    zip -m jam_${KEY}_${VALUE}.zip jam

    if [[ "$KEY" == "darwin" ]]
    then
        xcrun notarytool submit jam_${KEY}_${VALUE}.zip --keychain-profile "AC_PASSWORD" --wait
    fi
done

mkdir -p ./build/static
mv jam* ./build/static

echo "Done!"
