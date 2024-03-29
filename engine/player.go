package engine

import (
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/zsrv/rt5-server-go/util"
	"github.com/zsrv/rt5-server-go/util/packet"
)

const (
	MessageTypeGame        = 0
	MessageTypePublic      = 1
	MessageTypeTrade       = 4
	MessageTypePrivateTo   = 6
	MessageTypePrivateFrom = 7
	MessageTypeAssist      = 10
	MessageTypeDevConsole  = 99
)

type Player struct {
	Client *Client

	FirstLoad    bool
	Reconnecting bool
	Loaded       bool
	Loading      bool
	Appearance   *packet.Packet
	Placement    bool
	VerifyID     int

	ID         int
	Username   string
	WindowMode uint8

	World *World

	LastPos *util.Position

	Pos *util.Position
}

func NewPlayer(client *Client) *Player {
	return &Player{
		Client: client,

		FirstLoad:    true,
		Reconnecting: false,
		Loaded:       false,
		Loading:      false,
		Appearance:   nil,
		Placement:    false,
		VerifyID:     1,

		ID:         1,
		Username:   "",
		WindowMode: 0,

		LastPos: util.NewPosition(0, 0, 0),

		// make-over mage: 2925, 3323, 0
		// varrock square: 3213, 3443
		Pos: util.NewPosition(3162, 3490, 0),
	}
}

func (p *Player) Tick() {
	if !p.Loaded && !p.Loading {
		p.Loading = true

		if p.Reconnecting {
			var response packet.PacketBit
			response.P2(0)
			start := response.Len() // offset

			// INIT_GPI

			response.AccessBits()
			response.PBit(30, p.Pos.HighRes())
			p.LastPos.Clone(p.Pos)

			for i := 1; i < 2048; i++ {
				if p.ID == i {
					continue
				}
				response.PBit(18, 0)
			}
			response.AccessBytes()

			response.PSize2(response.Len() - start)
			respBytes := response.Bytes()
			//util.DebugfBytes(&p.Client.Server.Logger, "Tick() Reconnecting queue", respBytes)
			p.Client.Queue(respBytes, false)
		} else if p.FirstLoad {
			// TODO: something's wrong in here
			var response packet.PacketBit
			response.P1(98)
			response.P2(0)
			start := response.Len()

			// INIT_GPI

			response.AccessBits()
			response.PBit(30, p.Pos.HighRes())
			p.LastPos.Clone(p.Pos)

			for i := 1; i < 2048; i++ {
				if p.ID == i {
					continue
				}
				response.PBit(18, 0)
			}
			response.AccessBytes()

			// REBUILD_NORMAL

			response.IP2(uint16(p.Pos.ZoneX()))
			response.P2(uint16(p.Pos.ZoneZ()))
			response.P1(uint8(p.Pos.BAIndex))
			response.P1Alt2(0)

			for mapsquareX := (p.Pos.ZoneX() - (p.Pos.BASizeX >> 4)) >> 3; mapsquareX <= (p.Pos.ZoneX()+(p.Pos.BASizeX>>4))>>3; mapsquareX++ {
				for mapsquareZ := (p.Pos.ZoneZ() - (p.Pos.BASizeZ >> 4)) >> 3; mapsquareZ <= (p.Pos.ZoneZ()+(p.Pos.BASizeZ>>4))>>3; mapsquareZ++ {
					xtea, found := util.GetXTEA(mapsquareX, mapsquareZ)
					if found {
						for i := 0; i < len(xtea.Key); i++ {
							// TODO: converting signed to unsigned!!
							response.P4(uint32(xtea.Key[i]))
						}
					} else {
						for i := 0; i < 4; i++ {
							response.P4(0)
						}
					}
				}
			}

			response.PSize2(response.Len() - start)
			respBytes := response.Bytes()
			//util.DebugfBytes(&p.Client.Server.Logger, "Tick() FirstLoad queue", respBytes)
			p.Client.Queue(respBytes, true)
		}

		if p.FirstLoad {
			if p.IsClientResizable() {
				p.OpenGameFrame(746) // fixed?
			} else {
				p.OpenGameFrame(548) // resizable?
			}
			p.OpenChatBox(752)

			p.OpenTab(0, 884)
			p.OpenTab(1, 320)
			p.OpenTab(2, 190)
			p.OpenTab(3, 259)
			p.OpenTab(4, 149)
			p.OpenTab(5, 387)
			p.OpenTab(6, 271)
			p.OpenTab(7, 192)
			p.OpenTab(8, 891)
			p.OpenTab(9, 550)
			p.OpenTab(10, 551)
			p.OpenTab(11, 589)
			p.OpenTab(12, 261)
			p.OpenTab(13, 464)
			p.OpenTab(14, 187)
			p.OpenTab(15, 34)
			p.OpenTab(16, 182)

			if !p.Reconnecting {
				p.MessageGame("Welcome to RuneScape.", MessageTypeGame, "", "")
			}
		}

		p.FirstLoad = false
		p.Loading = false
		p.Loaded = true
	}

	// player info
	if p.Loaded {
		var response packet.PacketBit
		var updateBlock packet.Packet

		response.P1(72)
		response.P2(0)
		start := response.Len() // offset

		p.ProcessActivePlayers(&response, &updateBlock, true)
		p.ProcessActivePlayers(&response, &updateBlock, false)
		p.ProcessInactivePlayers(&response, &updateBlock, true)
		p.ProcessInactivePlayers(&response, &updateBlock, false)
		x := updateBlock.Bytes()
		response.PData(x, len(x))

		response.PSize2(response.Len() - start) // offset
		respBytes := response.Bytes()
		//util.DebugfBytes(&p.Client.Server.Logger, "Tick() player info queue", respBytes)
		p.Client.Queue(respBytes, true)
	}
}

func (p *Player) IsClientResizable() bool {
	// 1 = fixed, 2 = resizable, 3 = fullscreen
	return p.WindowMode > 1
}

func (p *Player) ProcessActivePlayers(buf *packet.PacketBit, updateBlock *packet.Packet, nsn0 bool) {
	buf.AccessBits()
	// TODO: this is supposed to loop, and "nsn0" is supposed to check against a player flag to skip
	if nsn0 {
		needsMaskUpdate := p.Appearance == nil
		needsUpdate := p.Placement || needsMaskUpdate

		if needsUpdate {
			buf.PBit(1, 1)
		} else {
			buf.PBit(1, 0)
		}

		if needsUpdate {
			if needsMaskUpdate {
				buf.PBit(1, 1)
			} else {
				buf.PBit(1, 0)
			}

			buf.PBit(2, 0) // no further update

			//if p.Placement {
			//	buf.PBit(2, 3) // teleport
			//	buf.PBit(1, 1) // full location update
			//	buf.PBit(30, p.pos.Z | p.pos.X << 14 | p.pos.Plane << 28)
			//}
		}

		if needsMaskUpdate {
			p.AppendUpdateBlock(updateBlock)
		}
	}
	buf.AccessBytes()
}

func (p *Player) ProcessInactivePlayers(buf *packet.PacketBit, updateBlock *packet.Packet, nsn2 bool) {
	buf.AccessBits()
	// TODO: "nsn2" is supposed to check against a player flag to skip
	if nsn2 {
		for i := 1; i < 2048; i++ {
			if p.ID == i {
				continue
			}

			buf.PBit(1, 0)
			buf.PBit(2, 0)
		}
	}
	buf.AccessBytes()
}

func (p *Player) GenerateAppearance() {
	var buf packet.Packet

	buf.P1(0)   // flags
	buf.P1(255) // title-related (was -1)
	buf.P1(255) // pkIcon (was -1)
	buf.P1(255) // prayerIcon

	//for i := 0; i < 12; i++ {
	//	buf.P1(0) // body
	//}

	// hat, cape, amulet, weapon, chest, shield, arms, legs, hair, wrists, hands, feet, beard
	body := []uint8{255, 255, 255, 255, 18, 255, 26, 36, 0, 33, 42, 10}
	for i := 0; i < len(body); i++ {
		if body[i] == 255 {
			buf.P1(0)
		} else {
			body[i] += uint8(math.Floor(rand.Float64() * 2))
			buf.P2(uint16(body[i]) | 0x100)
		}
	}

	for i := 0; i < 5; i++ {
		buf.P1(uint8(math.Floor(rand.Float64() * 4))) // color
	}

	buf.P2(1426) // bas id
	buf.PJStr(p.Username)
	buf.P1(3)  // combat level
	buf.P2(33) // total level
	buf.P1(0)  // sound radius

	//p.Appearance = packet.New(5000)
	p.Appearance = new(packet.Packet)
	x := buf.Bytes()
	p.Appearance.IPData(x, len(x))
}

func (p *Player) AppendUpdateBlock(buf *packet.Packet) {
	var flags uint8 = 0

	if p.Appearance == nil {
		p.GenerateAppearance()
		flags |= 0x1
	}

	buf.P1(flags)

	if flags&0x1 == 1 {
		buf.P1Alt3(uint8(p.Appearance.Len()))
		l := p.Appearance.Len()
		buf.PData(p.Appearance.Bytes(), l)
	}
}

func (p *Player) ProcessIn() {
	decoded := p.Client.DecodeIn()

	for _, v := range decoded {
		switch v.ID {
		//case 78: // MOVE_GAMECLICK
		//	ctrlClick := v.Data.G1() // g1add
		//
		//	x := v.Data.G2()
		//	z := v.Data.G2Alt1()
		//
		//	p.Pos.X = int(x)
		//	p.Pos.Z = int(z)
		//
		//	if ctrlClick != 0 {
		//		p.Placement = true
		//	}
		case util.ClientProtClientCheat:
			_ = v.Data.G1() // tele :=

			cmd := strings.ToLower(v.Data.GJStr())
			args := strings.Split(cmd, " ")

			cmd = args[0]
			args = args[1:]

			if cmd == "logout" {
				p.Logout()
			}
		default:
			p.Client.Server.Logger.Warn("unhandled packet", "packetID", v.ID)
		}
	}
}

// events

func (p *Player) OpenChatBox(interfaceID uint16) {
	if interfaceID == 752 {
		if p.IsClientResizable() {
			p.OpenInterface(746, 15, 751, 3)
			p.OpenInterface(746, 18, 752, 3)
		} else {
			p.OpenInterface(548, 20, 751, 3)
			p.OpenInterface(548, 142, 752, 3)
		}

		if p.IsClientResizable() {
			p.OpenInterface(752, 9, 137, 3)
		}
	}
}

func (p *Player) OpenTab(tabID uint16, interfaceID uint16) {
	if p.IsClientResizable() {
		p.OpenInterface(746, 33+tabID, interfaceID, 3)
	} else {
		p.OpenInterface(548, 152+tabID, interfaceID, 3)
	}
}

// encoders

func (p *Player) Logout() {
	var response packet.Packet
	response.P1(58)
	respBytes := response.Bytes()
	util.DebugfBytes(&p.Client.Server.Logger, "Logout() queue", respBytes)
	p.Client.Queue(respBytes, true)
}

func (p *Player) MessageGame(msg string, msgType uint8, msg2 string, msg3 string) {
	var response packet.Packet
	response.P1(99)
	response.P1(0)
	start := response.Len() // offset

	response.PSmart(uint16(msgType))
	response.P4(uint32(time.Now().UnixMilli() / 1000))

	var more uint8 = 0
	if msg2 != "" {
		more |= 0x1
	}

	if msg3 != "" {
		more |= 0x2
	}

	response.P1(more)
	if more&0x1 != 0 {
		response.PJStr(msg2)
	}

	if more&0x2 != 0 {
		response.PJStr(msg3)
	}

	response.PJStr(msg)

	response.PSize1(response.Len() - start)

	respBytes := response.Bytes()
	util.DebugfBytes(&p.Client.Server.Logger, "MessageGame() queue", respBytes)
	p.Client.Queue(respBytes, true)
}

func (p *Player) OpenGameFrame(interfaceId uint16) {
	var response packet.Packet
	response.P1(93)

	response.P1(0)
	response.IP2(interfaceId)
	response.IP2(uint16(p.VerifyID))
	p.VerifyID += 1

	respBytes := response.Bytes()
	util.DebugfBytes(&p.Client.Server.Logger, "OpenGameFrame() queue", respBytes)
	p.Client.Queue(respBytes, true)
}

func (p *Player) OpenInterface(windowID uint16, componentId uint16, interfaceId uint16, flags uint8) {
	var response packet.Packet
	response.P1(52)

	response.P2Alt2(uint16(p.VerifyID))
	p.VerifyID += 1
	response.P1Alt3(flags)
	response.P2Alt1(componentId)
	response.P2Alt1(windowID)
	response.P2(interfaceId)

	respBytes := response.Bytes()
	util.DebugfBytes(&p.Client.Server.Logger, "OpenInterface() queue", respBytes)
	p.Client.Queue(respBytes, true)
}
