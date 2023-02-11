# opa-go-service

# Start
    $ opa-go-service server

# Examples
--------

To evaluate a simple query:

    $ curl -X POST http://localhost:8080/execute -H 'Content-Type: application/json' -H 'Accept: application/json' --data '{"query":"x := 1; y := 2; x < y"}'

To evaluate a query against JSON data:

    $ curl -X POST http://localhost:8080/execute -H 'Content-Type: application/json' -H 'Accept: application/json' --data '{"query":"result = data", "data": "{\"test\":1}"}'

To evaluate a query against JSON data custom result:

    $ curl -X POST http://localhost:8080/execute -H 'Content-Type: application/json' -H 'Accept: application/json' --data '{"resultPath":"{$..Bindings.result}", "query":"result = data", "data": "{\"test\":1}"}'

To evaluate a query against Input custom result:
    
    $ curl -X POST http://localhost:8080/execute -H 'Content-Type: application/json' -H 'Accept: application/json' --data '{"resultPath":"{\$..Bindings.result}", "query":"result = input", "input": "{\"test\":1}"}'

To evaluate a query against IsCache:
    
    $ curl -X POST http://localhost:8080/execute -H 'Content-Type: application/json' -H 'Accept: application/json' --data '{"resultPath":"{\$..Bindings.result}", "query":"result = input", "input": "{\"test\":1}", "isCache": true}'
 First
 
    2023/02/11 15:13:23 [127.0.0.1] [1.311ms] POST /execute http 200 48
 
 Cache
 
    2023/02/11 15:13:33 [127.0.0.1] [0.271ms] POST /execute http 200 48