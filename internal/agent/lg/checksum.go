package lg

// CalcLGAPChecksum 은 LGAP 체크섬을 계산한다: (모든 바이트의 합 % 256) XOR 0x55.
// 체크섬 바이트 자체는 계산에 포함하지 않는다.
func CalcLGAPChecksum(data []byte) byte {
	var sum byte
	for _, b := range data {
		sum += b
	}
	return sum ^ 0x55
}

// VerifyChecksum 은 데이터의 마지막 바이트가 유효한 체크섬인지 검증한다.
// 데이터가 2바이트 미만이면 false 를 반환한다.
func VerifyChecksum(data []byte) bool {
	if len(data) < 2 {
		return false
	}
	expected := CalcLGAPChecksum(data[:len(data)-1])
	return expected == data[len(data)-1]
}
