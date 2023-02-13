// Copyright 2023 Honyrik.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	pb "github.com/Honyrik/opa-go-service/grpc"
	myUtil "github.com/Honyrik/opa-go-service/util"
	"github.com/erikdubbelboer/fasthttp"
	routing "github.com/jackwhelpton/fasthttp-routing"
	"github.com/jackwhelpton/fasthttp-routing/access"
	"github.com/jackwhelpton/fasthttp-routing/content"
	"github.com/jackwhelpton/fasthttp-routing/fault"
	"github.com/jackwhelpton/fasthttp-routing/slash"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/storage/inmem"
	"github.com/open-policy-agent/opa/util"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"k8s.io/client-go/util/jsonpath"
)

type serverCommandParams struct {
	restPort string
	grpcPort string
}

type server struct {
	pb.UnimplementedApiServer
}

var cachePrepare = make(map[string]rego.PreparedEvalQuery)

func getPreparedEvalQuery(ctx context.Context, in *pb.ApiRequest) (rego.PreparedEvalQuery, error) {
	var strArray []string
	if in.Query == "" {
		return rego.PreparedEvalQuery{}, fmt.Errorf("Need query")
	}

	if in.Data != "" {
		strArray = append(strArray, in.Data)
	}
	if len(in.Packages) > 0 {
		strArray = append(strArray, in.Packages...)
	}
	strArray = append(strArray, in.Query)
	buf := &bytes.Buffer{}
	gob.NewEncoder(buf).Encode(strArray)
	md5SumBuf := md5.Sum(buf.Bytes())

	md5Sum := string(md5SumBuf[:])
	pq, exist := cachePrepare[md5Sum]

	if exist && in.IsCache {
		return pq, nil
	}

	regoArgs := []func(*rego.Rego){rego.Query(in.Query)}

	if in.Data != "" {
		var data map[string]interface{}
		err := util.Unmarshal([]byte(in.Data), &data)
		if err != nil {
			return pq, err
		}
		store := inmem.NewFromObject(data)
		regoArgs = append(regoArgs, rego.Store(store))
	}

	if len(in.Packages) > 0 {
		for index, data := range in.Packages {
			regoArgs = append(regoArgs, rego.Module(fmt.Sprint("rego_%d.rego", index), data))
		}
	}

	r := rego.New(regoArgs...)

	pq, resultErr := r.PrepareForEval(ctx)
	if resultErr != nil {
		return pq, resultErr
	}

	if in.IsCache {
		cachePrepare[md5Sum] = pq
	}
	return pq, nil
}

func ExecuteRego(ctx context.Context, in *pb.ApiRequest) (*pb.ApiResult, error) {
	pq, errPq := getPreparedEvalQuery(ctx, in)

	if errPq != nil {
		return &pb.ApiResult{
			IsSucces: false,
			Error:    fmt.Sprint("unable to prepare query: %v", errPq),
		}, nil
	}

	evalArgs := []rego.EvalOption{
		rego.EvalRuleIndexing(true),
		rego.EvalEarlyExit(true),
	}

	if in.Input != "" {
		var input interface{}
		err := util.Unmarshal([]byte(in.Input), &input)
		if err != nil {
			return &pb.ApiResult{
				IsSucces: false,
				Error:    fmt.Sprint("unable to parse input: %v", err),
			}, nil
		}
		evalArgs = append(evalArgs, rego.EvalInput(input))
	}
	result, resultErr := pq.Eval(ctx, evalArgs...)
	if resultErr != nil {
		return &pb.ApiResult{
			IsSucces: false,
			Error:    fmt.Sprint("Unable Eval: %v", resultErr),
		}, nil
	}

	if in.ResultPath == "" {
		res := myUtil.ResultSetTArrayMap(result)
		resJson, resJsonErr := json.Marshal(res)
		if resJsonErr != nil {
			return &pb.ApiResult{
				IsSucces: false,
				Error:    fmt.Sprint("Unable Json: %v", resJsonErr),
			}, nil
		}
		return &pb.ApiResult{
			IsSucces: true,
			Result:   string(resJson[:]),
		}, nil
	}

	parse := jsonpath.New("")
	parse.EnableJSONOutput(false)
	resultPathErr := parse.Parse(in.ResultPath)
	if resultPathErr != nil {
		return &pb.ApiResult{
			IsSucces: false,
			Error:    fmt.Sprint("Unable Prepare Result Path: %v", resultPathErr),
		}, nil
	}

	res := myUtil.ResultSetTArrayMap(result)
	w := new(strings.Builder)
	printErr := parse.Execute(w, res)
	if printErr != nil {
		return &pb.ApiResult{
			IsSucces: false,
			Error:    fmt.Sprint("Unable Find Result Path: ", printErr),
		}, nil
	}

	return &pb.ApiResult{
		IsSucces: true,
		Result:   w.String(),
	}, nil
}

func (s *server) Execute(ctx context.Context, in *pb.ApiRequest) (*pb.ApiResult, error) {
	return ExecuteRego(ctx, in)
}

func Execute(ctx *routing.Context) error {
	data := pb.ApiRequest{}
	err := json.Unmarshal(ctx.PostBody(), &data)
	if err != nil {
		ctx.Write(&pb.ApiResult{
			IsSucces: false,
			Error:    fmt.Sprint("Unable Post Data: %v", err),
		})
		return nil
	}
	ctxRego := context.Background()

	res, err := ExecuteRego(ctxRego, &data)

	if err != nil {
		ctx.Write(&pb.ApiResult{
			IsSucces: false,
			Error:    fmt.Sprint("Unable Execute Rego: %v", err),
		})
		return nil
	}

	ctx.Write(res)

	return nil
}

func init() {

	params := serverCommandParams{}

	evalCommand := &cobra.Command{
		Use:   "server",
		Short: "Rest/gPRC service",
		Long:  `Rest/gPRC service.`,
		Run: func(cmd *cobra.Command, args []string) {

			_, err := startServer(args, params, os.Stdout)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		},
	}

	evalCommand.Flags().StringVarP(&params.grpcPort, "grpc-port", "g", os.Getenv("GRPC_PORT"), "gRPC port")
	evalCommand.Flags().StringVarP(&params.grpcPort, "rest-port", "r", os.Getenv("REST_PORT"), "REST port")
	RootCommand.AddCommand(evalCommand)
}

func maxMessageSize() int {
	maxRequestBodySize := 100 * 1024 * 1024

	if os.Getenv("MAX_MESSAGE_SIZE") != "" {
		i, err := strconv.Atoi(os.Getenv("MAX_MESSAGE_SIZE"))
		if err != nil {
			log.Println(err)
		} else {
			maxRequestBodySize = i
		}
	}

	return maxRequestBodySize
}

func connextionTimeout() time.Duration {
	maxRequestBodySize, _ := time.ParseDuration("1200s")

	if os.Getenv("CONNECTION_TIMEOUT") != "" {
		i, err := time.ParseDuration(os.Getenv("CONNECTION_TIMEOUT"))
		if err != nil {
			log.Println(err)
		} else {
			return i
		}
	}

	return maxRequestBodySize
}

func startRest(restPort string, errChan chan error) {
	router := routing.New()
	router.Use(
		access.Logger(log.Printf),
		slash.Remover(fasthttp.StatusMovedPermanently),
		fault.Recovery(log.Printf),
		content.TypeNegotiator(content.JSON, content.XML),
	)
	router.Post("/execute", Execute)
	log.Printf("server Rest listening at %s", restPort)
	s := fasthttp.Server{
		Handler:            router.HandleRequest,
		Name:               "opa-rest",
		MaxRequestBodySize: maxMessageSize(),
		ReadTimeout:        connextionTimeout(),
		WriteTimeout:       connextionTimeout(),
	}
	if err := s.ListenAndServe(fmt.Sprintf(":%s", restPort)); err != nil {
		errChan <- fmt.Errorf("failed to serve: %v", err)
	}
}

func startGrpc(grpcPort string, errChan chan error) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		errChan <- fmt.Errorf("failed to listen: %v", err)
	}
	var opts []grpc.ServerOption
	opts = append(opts,
		grpc.MaxMsgSize(maxMessageSize()),
		grpc.ConnectionTimeout(connextionTimeout()),
	)
	s := grpc.NewServer(opts...)

	pb.RegisterApiServer(s, &server{})
	log.Printf("server grpc listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		errChan <- fmt.Errorf("failed to serve: %v", err)
	}
}

func startServer(args []string, params serverCommandParams, w io.Writer) (bool, error) {
	grpcPort := params.grpcPort
	if grpcPort == "" {
		grpcPort = "8000"
	}
	restPort := params.restPort
	if restPort == "" {
		restPort = "8080"
	}
	errChan := make(chan error)

	go startGrpc(grpcPort, errChan)
	go startRest(restPort, errChan)

	for {
		val, ok := <-errChan
		if ok != false {
			return false, val
		}
	}

	return true, nil
}
