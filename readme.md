https://www.youtube.com/watch?v=JuUAEYLkGbM

basic websocket server that exposes 2 endpoints

how to use this project

```
go run main.go
```

open up a console in a browser and run

```js
let socket = new WebSocket("ws://localhost:3000/ws");
socket.onmessage = (event) => console.log(event.data);
socket.send("oh hello");
```

to see the feed

```js
let socket = new WebSocket("ws://localhost:3000/orderbookfeed")
socket.onmessage = (event) => console.log(event.data);
```