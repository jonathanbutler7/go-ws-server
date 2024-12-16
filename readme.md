started out with this tutorial on youtube:
https://www.youtube.com/watch?v=JuUAEYLkGbM

then i expanded it to handle
- users
- chat rooms 
- users can join/leave rooms
- different websocket message types

in the future i would like to add
- a more efficient broadcast to room algorithm (avoid nested for loops)
- store data somewhere
- integrate nsq or kafka

i built a codesandbox ui to showcase the connections working in the browser
- https://codesandbox.io/p/sandbox/funny-bush-92222l?file=%2Findex.html%3A30%2C50
- the ui is kind of confusing if you think about it carefully
- but kinda works if you don't think about it too much

how to use this project

```
go run main.go
```

to see the "chat" work, open up 2 consoles in a browser and run

```js
let socket = new WebSocket("ws://localhost:3000/ws/?userID=123&roomId=A");
socket.onmessage = (event) => console.log(event.data);
// use socket.send to send chat messages between the two connections
socket.send(JSON.stringify({type: "message",content: "oh hi"}))
```
