# Reverse Proxy

This project serves mainly the goal to get to know a bit of golang.

## General

Client -> Reverse Proxy -> GameServer (actual resources)

Client authenticates to proxy

Proxy finds server or starts new one (server selection)

Proxy forwards auth

Server can send stuff (config) back to client via proxy

Connection can be closed from both sides

## Stuff to consider

- TCP connections
    - Framing of messages
- Filter bad actors (can be filtered by rules, but for now maybe just allow all)
- Authentication (probably also allow everyone for now)
- Server assignments
    - How do we keep track of servers? SQLite? Something simpler?
- What if Client closes connection -> Server should close connection

Need simulation to test it

## Start with
- Just simple connection TCP (socket) client server with TCP 
- Auth for clients
- forward data to game server and send data back (some config)
- store data on proxy

## What are we tackeling
- Load balancing
- Protection from attacks?
