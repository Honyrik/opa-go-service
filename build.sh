docker build -t xcgo:latest --build-arg GO_VERSION=go1.20 .
docker run --name opa-build -v $(pwd):/root/src --rm -it xcgo:latest ls -l /root/src/dist