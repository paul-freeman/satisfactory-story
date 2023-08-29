package recipes

import (
	"fmt"
	"strings"
)

type Producer string

const (
	Assembler    Producer = "Assembler"
	Constructor  Producer = "Constructor"
	Manufacturer Producer = "Manufacturer"
	Refinery     Producer = "Refinery"
	Smelter      Producer = "Smelter"
	Foundry      Producer = "Foundry"
	Packager     Producer = "Packager"
	Blender      Producer = "Blender"
	Collider     Producer = "Collider"

	BuildGun Producer = "BuildGun"
	Workshop Producer = "Workshop"

	NullProducer Producer = "NullProducer"

	buildableFactory = "/Game/FactoryGame/Buildable/Factory"
)

func (p Producer) String() string {
	return string(p)
}

func (p *Producer) UnmarshalJSON(b []byte) error {
	if p == nil {
		return fmt.Errorf("cannot unmarshal into nil pointer")
	}
	s := string(b)
	if s == "\"\"" {
		*p = NullProducer
		return nil
	}
	if !isBuildableFactory(s) {
		if isProducerType(s, BuildGun) {
			*p = BuildGun
		} else if isProducerType(s, Workshop) {
			*p = Workshop
		} else {
			return fmt.Errorf("unknown non buildable factory: %s", s)
		}
		return nil
	}
	if isProducerType(s, Assembler) {
		*p = Assembler
	} else if isProducerType(s, Constructor) {
		*p = Constructor
	} else if isProducerType(s, Manufacturer) {
		*p = Manufacturer
	} else if isProducerType(s, Refinery) {
		*p = Refinery
	} else if isProducerType(s, Smelter) {
		*p = Smelter
	} else if isProducerType(s, Foundry) {
		*p = Foundry
	} else if isProducerType(s, Packager) {
		*p = Packager
	} else if isProducerType(s, Blender) {
		*p = Blender
	} else if isProducerType(s, Collider) {
		*p = Collider
	} else {
		return fmt.Errorf("unknown factory: %s", s)
	}
	return nil
}

func isBuildableFactory(s string) bool {
	return strings.Contains(s, buildableFactory)
}

func isProducerType(s string, t Producer) bool {
	return strings.Contains(s, t.String())
}
