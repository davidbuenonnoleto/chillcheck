package ble

// Advertisement is a decoder-friendly view of a BLE broadcast. scan.go builds
// these from the platform BLE library so the decoders below stay pure and testable.
type Advertisement struct {
	MAC              string
	LocalName        string
	RSSI             int16
	ServiceData      map[uint16][]byte // 16-bit service UUID -> data
	ManufacturerData map[uint16][]byte // company ID -> data
}

// Decoder turns an advertisement into a temperature in Celsius. Add a new sensor
// family by implementing this and appending it to Decoders().
type Decoder interface {
	Name() string
	DecodeC(adv Advertisement) (tempC float64, ok bool)
}

// Decoders returns the decoders tried, in order, for each advertisement.
func Decoders() []Decoder {
	return []Decoder{XiaomiESS{}, BTHomeV2{}}
}

// ---------- Xiaomi LYWSD03MMC with ATC1441 / pvvx firmware ----------
// Broadcasts under the Environmental Sensing service (0x181A). Cheap, hackable,
// and the most common DIY cold-chain sensor.

type XiaomiESS struct{}

func (XiaomiESS) Name() string { return "xiaomi_ess" }

func (XiaomiESS) DecodeC(adv Advertisement) (float64, bool) {
	d, ok := adv.ServiceData[0x181A]
	if !ok {
		return 0, false
	}
	switch len(d) {
	case 13: // ATC1441: big-endian, temp at [6:8] in 0.1 C
		t := int16(uint16(d[6])<<8 | uint16(d[7]))
		return float64(t) / 10.0, true
	case 15: // pvvx custom: little-endian, temp at [6:8] in 0.01 C
		t := int16(uint16(d[7])<<8 | uint16(d[6]))
		return float64(t) / 100.0, true
	}
	return 0, false
}

// ---------- BTHome v2 (unencrypted) ----------
// Open standard (service UUID 0xFCD2) used by pvvx firmware, ESPHome, Shelly,
// b-parasite and others. We walk the measurement objects to find temperature.

type BTHomeV2 struct{}

func (BTHomeV2) Name() string { return "bthome_v2" }

func (BTHomeV2) DecodeC(adv Advertisement) (float64, bool) {
	d, ok := adv.ServiceData[0xFCD2]
	if !ok || len(d) < 1 {
		return 0, false
	}
	info := d[0]
	if info&0x01 != 0 {
		return 0, false // encrypted; not supported here
	}
	i := 1
	for i < len(d) {
		id := d[i]
		n, known := bthomeObjLen(id)
		if !known || i+1+n > len(d) {
			return 0, false // can't safely skip an unknown object
		}
		body := d[i+1 : i+1+n]
		if id == 0x02 { // temperature: int16 LE, factor 0.01
			t := int16(uint16(body[0]) | uint16(body[1])<<8)
			return float64(t) * 0.01, true
		}
		i += 1 + n
	}
	return 0, false
}

// bthomeObjLen returns the data length for common BTHome v2 object IDs. Enough
// to reach the temperature object on typical sensors.
func bthomeObjLen(id byte) (int, bool) {
	switch id {
	case 0x00, 0x01, 0x10: // packet id, battery %, generic boolean/power
		return 1, true
	case 0x02, 0x03, 0x0C, 0x12, 0x13, 0x14: // temp, humidity, voltage, co2, tvoc, moisture
		return 2, true
	case 0x04, 0x05: // pressure, illuminance
		return 3, true
	}
	return 0, false
}
