package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"sync/atomic"
	"time"

	"epix.pw/rinha/pb"

	bolt "go.etcd.io/bbolt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type rinhaDBServer struct {
	db     *bolt.DB
	serial *atomic.Int32
	*pb.UnimplementedRinhaDBServer
}

func (rss *rinhaDBServer) Close() {
	rss.db.Close()
}

func (rss *rinhaDBServer) Start(db_path string) error {
	db, err := bolt.Open(db_path, 0600, nil)
	db.NoSync = true
	if err != nil {
		return err
	}
	rss.db = db
	rss.serial = &atomic.Int32{}
	fmt.Println("OPEN")
	return nil
}

func (rss *rinhaDBServer) MakeKey(name string, id int32) []byte {
	return []byte(fmt.Sprintf("%s_%011d", name, id))
}

func (rss *rinhaDBServer) Next() int32 {
	return rss.serial.Add(1)
}

func (rss *rinhaDBServer) GetBucketForCliente(tx *bolt.Tx, cliente_id int32) (*bolt.Bucket, error) {
	if cliente_id <= 0 {
		return nil, fmt.Errorf("Cliente ID não fornecido")
	}
	bucket_key := rss.MakeKey("bucket", cliente_id)
	return tx.CreateBucketIfNotExists(bucket_key)
}

func (rss *rinhaDBServer) AddCliente(ctx context.Context, cliente *pb.Cliente) (*pb.AddClienteResponse, error) {

	err := rss.db.Update(func(tx *bolt.Tx) error {
		bkt, err := rss.GetBucketForCliente(tx, cliente.Id)
		if err != nil {
			return err
		}

		value, err := proto.Marshal(cliente)
		if err != nil {
			return err
		}

		err = bkt.Put([]byte("cliente"), value)
		if err != nil {
			return err
		}
		return nil

	})

	return &pb.AddClienteResponse{
		Success: (err == nil),
	}, err
}

func (rss *rinhaDBServer) AddTransacao(ctx context.Context, req *pb.AddTransacaoRequest) (*pb.Saldo, error) {

	cliente := &pb.Cliente{}
	saldo := &pb.Saldo{}

	err := rss.db.Update(func(tx *bolt.Tx) error {
		bkt, err := rss.GetBucketForCliente(tx, req.ClienteId)
		if err != nil {
			return status.Error(codes.Internal, "Erro ao obter bucket de cliente")
		}

		key := []byte("cliente")

		clienteBytes := bkt.Get(key)
		if len(clienteBytes) == 0 {
			return status.Error(codes.NotFound, "Cliente inexistente")
		}

		err = proto.Unmarshal(clienteBytes, cliente)
		if err != nil {
			return status.Error(codes.Internal, "Erro ao converter bytes de cliente")
		}

		var delta int32
		switch req.Transacao.Tipo {
		case "d":
			delta = -req.Transacao.Valor
		case "c":
			delta = req.Transacao.Valor
		default:
			return status.Error(codes.InvalidArgument, "Tipo de transação inválido")
		}

		if (cliente.SaldoCorrente + delta) < -cliente.Limite {
			return status.Error(codes.InvalidArgument, fmt.Sprintf("Não há saldo suficiente para esta operação (saldo: %d, operacao: %d, limite: %d", cliente.SaldoCorrente, delta, cliente.Limite))
		}

		cliente.SaldoCorrente += delta

		if len(req.Transacao.Descricao) > 10 || len(req.Transacao.Descricao) < 1 {
			return status.Error(codes.InvalidArgument, "Descrição longa demais")
		}

		transacaoBytes, err := proto.Marshal(req.Transacao)
		if err != nil {
			return status.Error(codes.Internal, "Erro ao converter bytes de transação")
		}

		serial, err := bkt.NextSequence()
		if err != nil {
			return status.Error(codes.Internal, "Erro ao obter serial")
		}

		transacaoKey := rss.MakeKey("t", int32(serial))
		err = bkt.Put(transacaoKey, transacaoBytes)
		if err != nil {
			return status.Error(codes.Internal, "Erro ao gravar bytes de transação")
		}

		clienteBytes, err = proto.Marshal(cliente)
		err = bkt.Put([]byte("cliente"), clienteBytes)
		if err != nil {
			return status.Error(codes.Internal, "Erro ao gravar bytes de cliente")
		}

		saldo.Limite = cliente.Limite
		saldo.Total = cliente.SaldoCorrente

		return nil
	})

	if err != nil {
		return nil, err
	}

	return saldo, nil
}

func (rss *rinhaDBServer) GetExtrato(ctx context.Context, extratoRequest *pb.ExtratoRequest) (*pb.Extrato, error) {

	saldo := &pb.Saldo{}
	cliente := &pb.Cliente{}
	transacoes := make([]*pb.Transacao, 0)

	err := rss.db.View(func(tx *bolt.Tx) error {
		if extratoRequest.ClienteId <= 0 {
			fmt.Println("AQUI 1")
			return status.Error(codes.InvalidArgument, "Cliente invalido")
		}

		bucket_key := rss.MakeKey("bucket", extratoRequest.ClienteId)
		bkt := tx.Bucket(bucket_key)
		if bkt == nil {
			fmt.Println("AQUI 3")
			return status.Error(codes.NotFound, "Cliente inexistente")
		}

		key := []byte("cliente")

		clienteBytes := bkt.Get(key)
		if len(clienteBytes) == 0 {
			fmt.Println("AQUI 3")
			return status.Error(codes.NotFound, "Cliente inexistente")
		}

		err := proto.Unmarshal(clienteBytes, cliente)
		if err != nil {
			return status.Error(codes.Internal, "Erro ao converter bytes de cliente")
		}

		saldo.Total = cliente.SaldoCorrente
		saldo.Limite = cliente.Limite
		saldo.DataExtrato = time.Now().Format(time.RFC3339)

		c := bkt.Cursor()

		i := 0
		for k, v := c.Last(); k != nil && i < 10; k, v = c.Prev() {

			if string(k) == "cliente" {
				continue
			}

			t := &pb.Transacao{}
			err = proto.Unmarshal(v, t)
			if err != nil {
				return status.Error(codes.Internal, "Erro ao converter bytes de transação")
			}
			transacoes = append(transacoes, t)
			i++
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &pb.Extrato{
		Saldo:             saldo,
		UltimasTransacoes: transacoes,
	}, nil
}

func main() {

	var db_path = flag.String("db", "/tmp/rinha.boltdb", "Database file path")

	flag.Parse()

	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", 9000))

	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	rdbs := &rinhaDBServer{}
	err = rdbs.Start(*db_path)
	if err != nil {
		log.Fatal(err)
	}
	pb.RegisterRinhaDBServer(grpcServer, rdbs)
	grpcServer.Serve(lis)

}
