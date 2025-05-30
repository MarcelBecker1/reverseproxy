# Reverse Proxy Idea (Primegean inspired)

## General

Client -> Reverse Proxy -> GameServer (actual resources)

Client authenticates to proxy

Proxy finds server or starts new one (server selection)

Proxy forwards auth

Server can send stuff (config) back to client via proxy

Connection can be closed from both sides

## Stuff to consider

- TCP connections (handshake, packet format)
    - Framing of messages as we want to use raw tcp
- Filter bad actors (can be filtered by rules, but for now maybe just allow all)
- Authentication (probably also allow everyone for now)
- Server assignments
    - How do we keep track of servers? SQLite? Something simpler?
- What if Client closes connection -> Server should close connection

Need simulation to test it

## What i want to figure out

- Do we actual need one Server and then seperate smaller game entities? GameServers in normal servers doesn't really make sense
- Do we have proxy then the matchmaking servers (multiple), proxy keeps track of matchmaking servers and matchmaking servers have game servers
-> isnt that just 2 reverse proxies with load balancing?


## Start with
- Just simple connection TCP (socket) client server with TCP 
- Auth for clients
- forward data to game server and send data back (some config)
    - probably difficult as it required packet framing
- database on proxy

## What are we tackeling
- Load balancing
- Protection from attacks?