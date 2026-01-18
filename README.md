# Razpravljalnica
Razpravljalnica je distribuirana spletna storitev za izmenjavo mnenj med uporabniki o različnih temah. Uporabniki lahko:
- ustvarijo račun in se prijavijo,
- dodajajo nove teme,
- objavljajo sporočila znotraj tem,
- všečkajo sporočila, pri čemer se beleži število všečkov,
- naročijo se na eno ali več tem in sproti prejmejo obvestila o novih sporočilih.

**Arhitektura** temelji na **verižni replikaciji**, kjer so strežniki razporejeni kot HEAD, TAIL in vmesni INTERMEDIATE nodi:
- HEAD: sprejema vse pisalne operacije (ustvarjanje uporabnikov, tem, objavljanje sporočil, všečkanje).
- TAIL: zagotavlja bralne operacije (pridobivanje seznamov tem in sporočil).
- INTERMEDIATE nodi: posredujejo in replikirajo pisalne operacije od HEAD proti TAIL.

Odjemalec se poveže z HEAD za pisanje in z TAIL za branje ter podpira tudi naročnine na teme, kjer strežnik sproti pošilja nova sporočila uporabniku.

Storitev je implementirana v programskem jeziku Go in uporablja gRPC za komunikacijo med odjemalcem in strežnikom. gRPC je implementiran tudi za interno komunikacijo med strežniki.

## Zagon aplikacije
### Zagon serverjev
```
go run main.go --port 50051 --role HEAD
go run main.go --port 50052 --role INTERMEDIATE --successor 50053
go run main.go --port 50053 --role TAIL
```
HEAD node vedno prvi, TAIL zadnji, vmesni node-i kot intermediate.
Vsak node pozna naslednjega za propagacijo pisalnih operacij.

### Zagon clienta
```
go run client.go --head localhost:50051 --tail localhost:50053
```
Odjemalec uporablja HEAD za pisanje in TAIL za branje.
Odjemalec lahko komunicira z verigo preko gRPC.

## Podprte operacije v clientu
| Ukaz                      | Opis                                                                            | Kje se izvaja                |
| ------------------------- | ------------------------------------------------------------------------------- | ---------------------------- |
| `1 - Create User`         | Ustvari novega uporabnika                                                       | HEAD                         |
| `2 - Create Topic`        | Ustvari novo temo                                                               | HEAD                         |
| `3 - List Topics`         | Prikaže seznam vseh tem                                                         | TAIL                         |
| `4 - Post Message`        | Objavi sporočilo v izbrani temi                                                 | HEAD                         |
| `5 - Get Messages`        | Prebere sporočila iz teme                                                       | TAIL                         |
| `6 - Like Message`        | Všečka sporočilo                                                                | HEAD                         |
| `7 - Subscribe to Topics` | Naroči se na nove dogodke v izbranih temah                                      | Izbrani node (load-balanced) |
| `8 - Get Cluster State`   | Prikaže trenutno stanje klastra (HEAD, TAIL, chain)                             | HEAD                         |
| `9 - Demo Mode`           | Samodejno izvede testne operacije (ustvari uporabnike, teme, sporočila, všečke) | HEAD/Tail                    |

#### Demo Mode
Samodejno:
- Ustvari uporabnike (Alice, Bob).
- Ustvari teme (Programiranje, Šport).
- Počaka na replikacijo.
- Objavi sporočila na HEAD.
- Prebere sporočila iz TAIL.
- Všečka sporočila.
- Preveri všečke iz TAIL.
- Prikaže stanje klastra.

## Konkreten primer uporabe
Primer za tri vozlišča.

Terminal 1 - HEAD:
```
go run *.go -role head -p 9876 -successor localhost:9877 -all localhost:9876,localhost:9877,localhost:9878
```

Terminal 2 - INTERMEDIATE:
```
go run *.go -role intermediate -p 9877 -successor localhost:9878 -all localhost:9876,localhost:9877,localhost:9878
```

Terminal 3 - TAIL:
```
go run *.go -role tail -p 9878 -all localhost:9876,localhost:9877,localhost:9878
```

Terminal 4 - Client:
```
go run *.go -head localhost:9876 -tail localhost:9878
go run *.go -head localhost:9876 -tail localhost:9878 -test
```
Da zalaufamo uporabniški vmesnik:
```
go run ui/main.go -head localhost:9876 -tail localhost:9878
```

Na WINDOWS namesto **.go* napišemo *main.go server.go ... .go*.

## OPIS
### main.go
- inicializira in zažene server.go
- Lahko glavni vstopni point za strežnik.
- Inicializira konfiguracijo (npr. seznam strežnikov v verigi).
- Zažene gRPC strežnik (server.go).

### server.go
- implementira interface iz razpravljalnica_grpc.pb.go
- Implementacija gRPC server interface (metode WritePost, GetPosts, …).
- Logika verižne replikacije: pošiljanje pisalnih zahtev do naslednjega strežnika, vračanje acknowledgment-a.
- Inicializira strežnik, posluša na določenem portu.

### client.go
- kliče metode preko stub-a iz razpravljalnica_grpc.pb.go, ki uporablja strukture iz razpravljalnica.pb.go
- odjemalec
- Ustvari gRPC connection, kliče metode preko stub-a.
- Lahko zaženeš več instanc za simulacijo več odjemalcev.

### client_advanced.go (testni odjemalec)
Testni odjemalec, ki avtomatizira preverjanje vseh funkcionalnosti Razpravljalnice:
- ustvarjanje uporabnikov in tem,
- objavljanje sporočil,
- branje sporočil z TAIL,
- všečkanje in preverjanje replikacije,
- naročnine in load-balancing.

### start_chain.sh
- avtomatizira zagon več strežnikov za verižno replikacijo.
- Skripta za zagon več instanc strežnikov v verigi.
- Nastavi port vsake instance in “nextServerAddr” za verigo.

### go.mod & go.sum
- go.mod: definira modul, verzije paketov (vključno z gRPC Go).
- go.sum: hash za preverjanje integritete paketov.

### razpravljalnica.proto
- generira razpravljalnica.pb.go (structs) + razpravljalnica_grpc.pb.go (stub)
- Definicija gRPC servisa in vseh metod (WritePost, GetPosts, SubscribeTopic, …).
- Definicija sporočil (request/response) z Protocol Buffers.
- To je “master definicija”, iz katere gRPC generira kodo za Go.

### razpravljalnica.pb.go
- Avtomatsko generirana Go koda za Protocol Buffers (strukturirana sporočila).
- Vsebuje Go strukture za request/response tipe.
- Odjemalec in strežnik jo uporabljata za serializacijo in deserializacijo podatkov.

### razpravljalnica_grpc.pb.go
- Avtomatsko generirana Go koda za gRPC stub-e.
- Vsebuje:
    - Server interface, ki ga implementiraš (WritePost(ctx, req), …).
    - Client stub, ki kliče metode kot lokalne funkcije, a dejansko gre RPC klic preko gRPC.
- Strežnik implementira interface, odjemalec uporablja stub.
