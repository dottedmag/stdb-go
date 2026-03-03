package main

//stdb:enum variants=Attack,Defense,Heal scope=Combat
type ActionType uint8

const (
	ActionTypeAttack  ActionType = 0
	ActionTypeDefense ActionType = 1
	ActionTypeHeal    ActionType = 2
)

//stdb:sumtype scope=Combat
type Effect interface{ isEffect() }

//stdb:variant of=Effect name=Damage
type EffectDamage struct {
	Amount uint32
}

//stdb:variant of=Effect name=Buff
type EffectBuff struct {
	Duration int64
	Power    float32
}
