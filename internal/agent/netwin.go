package agent

const (
	ifFlagFilterInterface = 0x02 // NDIS filter layer stacked on a real adapter
	ifOperNotPresent      = 6
	ifTypeLoopback        = 24
)

// ifRowUsable keeps real interfaces from the Windows interface table: no
// filter layers (WFP, QoS, ...), nothing that is not physically present,
// no loopback. Down interfaces stay; with no traffic they are hidden by
// the page's idle filter.
func ifRowUsable(flags uint8, operStatus, ifType uint32) bool {
	return flags&ifFlagFilterInterface == 0 && operStatus != ifOperNotPresent && ifType != ifTypeLoopback
}
