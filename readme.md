started out with this tutorial on youtube:
https://www.youtube.com/watch?v=JuUAEYLkGbM

then i expanded it to handle

- users
- chat rooms
- users can join/leave rooms
- different websocket message types

in the future i would like to add:

- [x] a more efficient broadcast to room algorithm (avoid nested for loops)
- [x] store data somewhere
  - [x] audit log in postgres would be a good place to start (easy)
- [ ] integrate nsq or kafka (or redis)
  - [ ] use case: multiple servers running
  - [ ] straightforward solution: publish every message to nsq (1 topic per room probably)
  - [ ] reason for message bus is horizontal scaling
- [ ] load test this

i built an html page ui to showcase the connections working in the browser

routes with nsq
1. at most, 1 subscriber processes each message
2. all subscribers should get this message and process it

- https://jonathanbutler7.github.io/
- the ui is kind of confusing if you think about it carefully
- but kinda works if you don't think about it too much
- you have to have this repo running locally in order for it to work

how to use this project

```
go run main.go
```

to see the "chat" work, open up 2 consoles in a browser and run

```js
let socket = new WebSocket("ws://localhost:3000/ws/?userId=123");
socket.onmessage = (event) => console.log(event.data);
// use socket.send to send chat messages between the two connections
socket.onopen = () => {
  // join a room with the user
  socket.send(
    JSON.stringify({
      type: "join",
      content: {
        userId: "123",
        roomId: "A",
      },
    })
  );

  // send a message to that room
  socket.send(
    JSON.stringify({
      type: "message",
      content: {
        text: "user 1",
        destination: "A",
      },
    })
  );
};
```
