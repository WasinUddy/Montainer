// Command bedrockclient is an offline, short-lived Bedrock compatibility
// canary. It joins an isolated CI server and can verify that a server-side
// teleport reaches the player as a movement packet.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sandertv/go-raknet"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func main() {
	address := flag.String("address", "montainer:19132", "Bedrock server address")
	username := flag.String("username", "MontainerCI", "offline player name")
	chat := flag.String("chat", "", "optional chat message sent after spawning")
	waitForTeleport := flag.Bool("wait-teleport", false, "wait for a server-side player teleport")
	expectedX := flag.Float64("expect-x", math.NaN(), "expected teleport X coordinate")
	expectedY := flag.Float64("expect-y", math.NaN(), "expected teleport Y coordinate")
	expectedZ := flag.Float64("expect-z", math.NaN(), "expected teleport Z coordinate")
	positionTolerance := flag.Float64("position-tolerance", 2, "allowed teleport coordinate difference")
	timeout := flag.Duration("timeout", 90*time.Second, "overall probe timeout")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	probeProtocol := minecraft.DefaultProtocol
	fmt.Printf("probe_support version=%s protocol=%d\n", probeProtocol.Ver(), probeProtocol.ID())
	serverVersion, serverProtocol, err := pingServer(ctx, *address)
	if err != nil {
		log.Fatalf("ping Bedrock: %v", err)
	}
	fmt.Printf("server_advertisement version=%s protocol=%d\n", serverVersion, serverProtocol)
	if serverProtocol != probeProtocol.ID() {
		log.Fatalf(
			"probe_protocol_unsupported server_version=%s server_protocol=%d probe_version=%s probe_protocol=%d",
			serverVersion, serverProtocol, probeProtocol.Ver(), probeProtocol.ID(),
		)
	}

	dialer := minecraft.Dialer{
		IdentityData:               login.IdentityData{DisplayName: *username},
		Protocol:                   probeProtocol,
		DisconnectOnInvalidPackets: true,
		DisconnectOnUnknownPackets: false,
	}
	connection, err := dialer.DialContext(ctx, "raknet", *address)
	if err != nil {
		log.Fatalf("dial Bedrock: %v", err)
	}
	defer connection.Close()
	if err := connection.DoSpawnContext(ctx); err != nil {
		log.Fatalf("spawn Bedrock player: %v", err)
	}

	game := connection.GameData()
	fmt.Printf(
		"spawned player=%q runtime_id=%d world=%q position=%.2f,%.2f,%.2f\n",
		connection.IdentityData().DisplayName,
		game.EntityRuntimeID,
		game.WorldName,
		game.PlayerPosition[0], game.PlayerPosition[1], game.PlayerPosition[2],
	)
	if *chat != "" {
		if err := connection.WritePacket(&packet.Text{
			TextType:   packet.TextTypeChat,
			SourceName: connection.IdentityData().DisplayName,
			Message:    *chat,
		}); err != nil {
			log.Fatalf("queue chat: %v", err)
		}
		if err := connection.Flush(); err != nil {
			log.Fatalf("send chat: %v", err)
		}
		fmt.Printf("chat_sent message=%q\n", *chat)
	}
	if !*waitForTeleport {
		return
	}
	if *positionTolerance < 0 {
		log.Fatal("position tolerance must not be negative")
	}
	expectedPosition := [3]float64{*expectedX, *expectedY, *expectedZ}
	checkPosition := !math.IsNaN(expectedPosition[0]) && !math.IsNaN(expectedPosition[1]) && !math.IsNaN(expectedPosition[2])

	if deadline, ok := ctx.Deadline(); ok {
		if err := connection.SetReadDeadline(deadline); err != nil {
			log.Fatalf("set packet read deadline: %v", err)
		}
	}
	for {
		incoming, err := connection.ReadPacket()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
				log.Fatal("timed out waiting for teleport")
			}
			var networkError net.Error
			if errors.As(err, &networkError) && networkError.Timeout() {
				log.Fatal("timed out waiting for teleport")
			}
			log.Fatalf("read Bedrock packet: %v", err)
		}
		movement, ok := incoming.(*packet.MovePlayer)
		if !ok || movement.EntityRuntimeID != game.EntityRuntimeID || movement.Mode != packet.MoveModeTeleport {
			continue
		}
		if checkPosition && (math.Abs(float64(movement.Position[0])-expectedPosition[0]) > *positionTolerance ||
			math.Abs(float64(movement.Position[1])-expectedPosition[1]) > *positionTolerance ||
			math.Abs(float64(movement.Position[2])-expectedPosition[2]) > *positionTolerance) {
			continue
		}
		fmt.Printf(
			"teleported runtime_id=%d position=%.2f,%.2f,%.2f cause=%d tick=%d\n",
			movement.EntityRuntimeID,
			movement.Position[0], movement.Position[1], movement.Position[2],
			movement.TeleportCause,
			movement.Tick,
		)
		return
	}
}

func pingServer(ctx context.Context, address string) (version string, protocolID int32, err error) {
	advertisement, err := raknet.PingContext(ctx, address)
	if err != nil {
		return "", 0, err
	}
	fields := strings.Split(string(advertisement), ";")
	if len(fields) < 4 || fields[0] != "MCPE" {
		return "", 0, fmt.Errorf("unexpected Bedrock advertisement %q", advertisement)
	}
	parsed, err := strconv.ParseInt(fields[2], 10, 32)
	if err != nil {
		return "", 0, fmt.Errorf("parse advertised protocol %q: %w", fields[2], err)
	}
	return fields[3], int32(parsed), nil
}
