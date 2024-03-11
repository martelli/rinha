Rinha de Backend 2023/Q1

Para contexto: https://github.com/zanfranceschi/rinha-de-backend-2024-q1

Tentando ir na contramão dos stacks usuais (PostgreSQL etc.), fiz uma versão
com **Go** com **fasthttp** no webserver e **BoltDB** conectado via **gRPC** como banco de dados.

Os testes foram feitos com **nginx** e rodando as instâncias de _httpserver_ (2x) e _boltdb_ localmente, sem Docker,
pois já havia passado o prazo de submissão, então não me importei que rodasse fora da minha máquina.

O resultado foi bem satisfatório:

![gatling](./misc/rinha.png)

Mexendo nos parâmetros do Gatling e colocando para 10k req/s, obtive o gráfico abaixo:


![gatling10k](./misc/rinha10k.png)

Disclaimer: o código podia estar mais limpo e organizado, mas foram 2 dias fazendo, então estava no modo: "teste passou, push!".

Agora é aguardar a terceira edição! :D
