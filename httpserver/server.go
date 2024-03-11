package main

import (
	"context"
	"flag"
	"fmt"
	"google.golang.org/grpc"
	"log"
	"strconv"
	"time"

	"epix.pw/rinha/pb"
	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

var conn *grpc.ClientConn

func clienteInitHandler(ctx *fasthttp.RequestCtx) {
	client := pb.NewRinhaDBClient(conn)

	limites := []int32{100000, 80000, 1000000, 10000000, 500000}
	cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for i, v := range limites {
		c := &pb.Cliente{
			Id:     int32(i + 1),
			Limite: v,
		}

		r, err := client.AddCliente(cctx, c)
		fmt.Println(r.String(), err)
	}
}

func transacoesHandler(ctx *fasthttp.RequestCtx) {

	t := &pb.Transacao{}
	err := jsonpb.UnmarshalString(string(ctx.PostBody()), t)
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		return
	}

	cliente_id, err := strconv.Atoi(ctx.UserValue("cliente_id").(string))
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
		return
	}

	t.RealizadaEm = time.Now().Format(time.RFC3339)

	cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := pb.NewRinhaDBClient(conn)

	tReq := &pb.AddTransacaoRequest{
		ClienteId: int32(cliente_id),
		Transacao: t,
	}

	answer, err := client.AddTransacao(cctx, tReq)
	if err != nil {
		var statusCode int
		switch status.Code(err) {
		case codes.InvalidArgument:
			statusCode = fasthttp.StatusUnprocessableEntity
		case codes.NotFound:
			statusCode = fasthttp.StatusNotFound
		default:
			fmt.Println(err)
			statusCode = fasthttp.StatusInternalServerError
		}
		ctx.SetStatusCode(statusCode)
		return
	}

	fmt.Fprintf(ctx, "{\"limite\": %d, \"saldo\": %d}", answer.Limite, answer.Total)

}

func extratoHandler(ctx *fasthttp.RequestCtx) {

	cliente_id, err := strconv.Atoi(ctx.UserValue("cliente_id").(string))
	if err != nil || cliente_id <= 0 {
		ctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
		return
	}

	cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := pb.NewRinhaDBClient(conn)
	answer, err := client.GetExtrato(cctx, &pb.ExtratoRequest{ClienteId: int32(cliente_id)})
	if err != nil {
		var statusCode int
		switch status.Code(err) {
		case codes.InvalidArgument:
			statusCode = fasthttp.StatusUnprocessableEntity
		case codes.NotFound:
			statusCode = fasthttp.StatusNotFound
		default:
			fmt.Println(err)
			statusCode = fasthttp.StatusInternalServerError
		}
		ctx.SetStatusCode(statusCode)
		return
	}

	m := protojson.MarshalOptions{EmitUnpopulated: true}
	j := m.Format(answer)
	fmt.Fprintf(ctx, "%s", j)
}

func main() {

	var err error

	serverAddress := flag.String("addr", "localhost:9000", "Database IP:Port")

	flag.Parse()

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	conn, err = grpc.Dial(*serverAddress, opts...)
	if err != nil {
		log.Fatal(err)
	}

	r := router.New()
	r.POST("/clientes/{cliente_id}/transacoes", transacoesHandler)
	r.GET("/clientes/{cliente_id}/extrato", extratoHandler)
	r.GET("/clientes/init", clienteInitHandler)

	fasthttp.ListenAndServe(":9001", r.Handler)
}
