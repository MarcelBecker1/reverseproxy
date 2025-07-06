# Reverse Proxy

This project serves as a learning oppurtinity. I want to explore go, networking with go as well as some basic reverse proxy functionalities.

## General Idea

**Client -> Reverse Proxy -> Game Server (actual resources)**

1. Client authenticates and authorizes to proxy
2. Proxy finds server or starts new one if none of the existing ones have capacity
3. Proxy forwards auth to game server
4. Game server sends back success via proxy to client
5. Proxy works as man in the middle and collects some data
6. Connection can be closed from both sides
7. Simulation for testing the functionalities with load and fuzzy testing

## What's Next

- Currently we only have very simple working simulations
- We need to implement more sophisticated tests and fuzzy testing
- Already added outline of state loading on which we can build on for more interesting tests

