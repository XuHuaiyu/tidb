// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: github.com/pingcap/tipb/go-mysqlx/Expect/mysqlx_expect.proto

/*
	Package Mysqlx_Expect is a generated protocol buffer package.

	Expect operations

	It is generated from these files:
		github.com/pingcap/tipb/go-mysqlx/Expect/mysqlx_expect.proto

	It has these top-level messages:
		Open
		Close
*/
package Mysqlx_Expect

import (
	"fmt"

	proto "github.com/golang/protobuf/proto"

	math "math"

	io "io"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

type Open_CtxOperation int32

const (
	// copy the operations from the parent Expect-block
	Open_EXPECT_CTX_COPY_PREV Open_CtxOperation = 0
	// start with a empty set of operations
	Open_EXPECT_CTX_EMPTY Open_CtxOperation = 1
)

var Open_CtxOperation_name = map[int32]string{
	0: "EXPECT_CTX_COPY_PREV",
	1: "EXPECT_CTX_EMPTY",
}
var Open_CtxOperation_value = map[string]int32{
	"EXPECT_CTX_COPY_PREV": 0,
	"EXPECT_CTX_EMPTY":     1,
}

func (x Open_CtxOperation) Enum() *Open_CtxOperation {
	p := new(Open_CtxOperation)
	*p = x
	return p
}
func (x Open_CtxOperation) String() string {
	return proto.EnumName(Open_CtxOperation_name, int32(x))
}
func (x *Open_CtxOperation) UnmarshalJSON(data []byte) error {
	value, err := proto.UnmarshalJSONEnum(Open_CtxOperation_value, data, "Open_CtxOperation")
	if err != nil {
		return err
	}
	*x = Open_CtxOperation(value)
	return nil
}
func (Open_CtxOperation) EnumDescriptor() ([]byte, []int) {
	return fileDescriptorMysqlxExpect, []int{0, 0}
}

type Open_Condition_ConditionOperation int32

const (
	// set the condition
	//
	// set, if not set
	// overwrite, if set
	Open_Condition_EXPECT_OP_SET Open_Condition_ConditionOperation = 0
	// unset the condition
	Open_Condition_EXPECT_OP_UNSET Open_Condition_ConditionOperation = 1
)

var Open_Condition_ConditionOperation_name = map[int32]string{
	0: "EXPECT_OP_SET",
	1: "EXPECT_OP_UNSET",
}
var Open_Condition_ConditionOperation_value = map[string]int32{
	"EXPECT_OP_SET":   0,
	"EXPECT_OP_UNSET": 1,
}

func (x Open_Condition_ConditionOperation) Enum() *Open_Condition_ConditionOperation {
	p := new(Open_Condition_ConditionOperation)
	*p = x
	return p
}
func (x Open_Condition_ConditionOperation) String() string {
	return proto.EnumName(Open_Condition_ConditionOperation_name, int32(x))
}
func (x *Open_Condition_ConditionOperation) UnmarshalJSON(data []byte) error {
	value, err := proto.UnmarshalJSONEnum(Open_Condition_ConditionOperation_value, data, "Open_Condition_ConditionOperation")
	if err != nil {
		return err
	}
	*x = Open_Condition_ConditionOperation(value)
	return nil
}
func (Open_Condition_ConditionOperation) EnumDescriptor() ([]byte, []int) {
	return fileDescriptorMysqlxExpect, []int{0, 0, 0}
}

// open an Expect block and set/unset the conditions that have to be fulfilled
//
// if any of the conditions fail, all enclosed messages will fail with
// a Mysqlx.Error message.
//
// :returns: :protobuf:msg:`Mysqlx::Ok` on success, :protobuf:msg:`Mysqlx::Error` on error
//
type Open struct {
	Op               *Open_CtxOperation `protobuf:"varint,1,opt,name=op,enum=Mysqlx.Expect.Open_CtxOperation,def=0" json:"op,omitempty"`
	Cond             []*Open_Condition  `protobuf:"bytes,2,rep,name=cond" json:"cond,omitempty"`
	XXX_unrecognized []byte             `json:"-"`
}

func (m *Open) Reset()                    { *m = Open{} }
func (m *Open) String() string            { return proto.CompactTextString(m) }
func (*Open) ProtoMessage()               {}
func (*Open) Descriptor() ([]byte, []int) { return fileDescriptorMysqlxExpect, []int{0} }

const Default_Open_Op Open_CtxOperation = Open_EXPECT_CTX_COPY_PREV

func (m *Open) GetOp() Open_CtxOperation {
	if m != nil && m.Op != nil {
		return *m.Op
	}
	return Default_Open_Op
}

func (m *Open) GetCond() []*Open_Condition {
	if m != nil {
		return m.Cond
	}
	return nil
}

type Open_Condition struct {
	ConditionKey     *uint32                            `protobuf:"varint,1,req,name=condition_key,json=conditionKey" json:"condition_key,omitempty"`
	ConditionValue   []byte                             `protobuf:"bytes,2,opt,name=condition_value,json=conditionValue" json:"condition_value,omitempty"`
	Op               *Open_Condition_ConditionOperation `protobuf:"varint,3,opt,name=op,enum=Mysqlx.Expect.Open_Condition_ConditionOperation,def=0" json:"op,omitempty"`
	XXX_unrecognized []byte                             `json:"-"`
}

func (m *Open_Condition) Reset()                    { *m = Open_Condition{} }
func (m *Open_Condition) String() string            { return proto.CompactTextString(m) }
func (*Open_Condition) ProtoMessage()               {}
func (*Open_Condition) Descriptor() ([]byte, []int) { return fileDescriptorMysqlxExpect, []int{0, 0} }

const Default_Open_Condition_Op Open_Condition_ConditionOperation = Open_Condition_EXPECT_OP_SET

func (m *Open_Condition) GetConditionKey() uint32 {
	if m != nil && m.ConditionKey != nil {
		return *m.ConditionKey
	}
	return 0
}

func (m *Open_Condition) GetConditionValue() []byte {
	if m != nil {
		return m.ConditionValue
	}
	return nil
}

func (m *Open_Condition) GetOp() Open_Condition_ConditionOperation {
	if m != nil && m.Op != nil {
		return *m.Op
	}
	return Default_Open_Condition_Op
}

// close a Expect block
//
// closing a Expect block restores the state of the previous Expect block
// for the following messages
//
// :returns: :protobuf:msg:`Mysqlx::Ok` on success, :protobuf:msg:`Mysqlx::Error` on error
type Close struct {
	XXX_unrecognized []byte `json:"-"`
}

func (m *Close) Reset()                    { *m = Close{} }
func (m *Close) String() string            { return proto.CompactTextString(m) }
func (*Close) ProtoMessage()               {}
func (*Close) Descriptor() ([]byte, []int) { return fileDescriptorMysqlxExpect, []int{1} }

func init() {
	proto.RegisterType((*Open)(nil), "Mysqlx.Expect.Open")
	proto.RegisterType((*Open_Condition)(nil), "Mysqlx.Expect.Open.Condition")
	proto.RegisterType((*Close)(nil), "Mysqlx.Expect.Close")
	proto.RegisterEnum("Mysqlx.Expect.Open_CtxOperation", Open_CtxOperation_name, Open_CtxOperation_value)
	proto.RegisterEnum("Mysqlx.Expect.Open_Condition_ConditionOperation", Open_Condition_ConditionOperation_name, Open_Condition_ConditionOperation_value)
}
func (m *Open) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Open) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.Op != nil {
		dAtA[i] = 0x8
		i++
		i = encodeVarintMysqlxExpect(dAtA, i, uint64(*m.Op))
	}
	if len(m.Cond) > 0 {
		for _, msg := range m.Cond {
			dAtA[i] = 0x12
			i++
			i = encodeVarintMysqlxExpect(dAtA, i, uint64(msg.Size()))
			n, err := msg.MarshalTo(dAtA[i:])
			if err != nil {
				return 0, err
			}
			i += n
		}
	}
	if m.XXX_unrecognized != nil {
		i += copy(dAtA[i:], m.XXX_unrecognized)
	}
	return i, nil
}

func (m *Open_Condition) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Open_Condition) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.ConditionKey == nil {
		return 0, new(proto.RequiredNotSetError)
	} else {
		dAtA[i] = 0x8
		i++
		i = encodeVarintMysqlxExpect(dAtA, i, uint64(*m.ConditionKey))
	}
	if m.ConditionValue != nil {
		dAtA[i] = 0x12
		i++
		i = encodeVarintMysqlxExpect(dAtA, i, uint64(len(m.ConditionValue)))
		i += copy(dAtA[i:], m.ConditionValue)
	}
	if m.Op != nil {
		dAtA[i] = 0x18
		i++
		i = encodeVarintMysqlxExpect(dAtA, i, uint64(*m.Op))
	}
	if m.XXX_unrecognized != nil {
		i += copy(dAtA[i:], m.XXX_unrecognized)
	}
	return i, nil
}

func (m *Close) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Close) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.XXX_unrecognized != nil {
		i += copy(dAtA[i:], m.XXX_unrecognized)
	}
	return i, nil
}

func encodeVarintMysqlxExpect(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func (m *Open) Size() (n int) {
	var l int
	_ = l
	if m.Op != nil {
		n += 1 + sovMysqlxExpect(uint64(*m.Op))
	}
	if len(m.Cond) > 0 {
		for _, e := range m.Cond {
			l = e.Size()
			n += 1 + l + sovMysqlxExpect(uint64(l))
		}
	}
	if m.XXX_unrecognized != nil {
		n += len(m.XXX_unrecognized)
	}
	return n
}

func (m *Open_Condition) Size() (n int) {
	var l int
	_ = l
	if m.ConditionKey != nil {
		n += 1 + sovMysqlxExpect(uint64(*m.ConditionKey))
	}
	if m.ConditionValue != nil {
		l = len(m.ConditionValue)
		n += 1 + l + sovMysqlxExpect(uint64(l))
	}
	if m.Op != nil {
		n += 1 + sovMysqlxExpect(uint64(*m.Op))
	}
	if m.XXX_unrecognized != nil {
		n += len(m.XXX_unrecognized)
	}
	return n
}

func (m *Close) Size() (n int) {
	var l int
	_ = l
	if m.XXX_unrecognized != nil {
		n += len(m.XXX_unrecognized)
	}
	return n
}

func sovMysqlxExpect(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozMysqlxExpect(x uint64) (n int) {
	return sovMysqlxExpect(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *Open) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowMysqlxExpect
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Open: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Open: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Op", wireType)
			}
			var v Open_CtxOperation
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMysqlxExpect
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= (Open_CtxOperation(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.Op = &v
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Cond", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMysqlxExpect
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthMysqlxExpect
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Cond = append(m.Cond, &Open_Condition{})
			if err := m.Cond[len(m.Cond)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipMysqlxExpect(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthMysqlxExpect
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			m.XXX_unrecognized = append(m.XXX_unrecognized, dAtA[iNdEx:iNdEx+skippy]...)
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *Open_Condition) Unmarshal(dAtA []byte) error {
	var hasFields [1]uint64
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowMysqlxExpect
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Condition: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Condition: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field ConditionKey", wireType)
			}
			var v uint32
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMysqlxExpect
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= (uint32(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.ConditionKey = &v
			hasFields[0] |= uint64(0x00000001)
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field ConditionValue", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMysqlxExpect
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthMysqlxExpect
			}
			postIndex := iNdEx + byteLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.ConditionValue = append(m.ConditionValue[:0], dAtA[iNdEx:postIndex]...)
			if m.ConditionValue == nil {
				m.ConditionValue = []byte{}
			}
			iNdEx = postIndex
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Op", wireType)
			}
			var v Open_Condition_ConditionOperation
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMysqlxExpect
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= (Open_Condition_ConditionOperation(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.Op = &v
		default:
			iNdEx = preIndex
			skippy, err := skipMysqlxExpect(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthMysqlxExpect
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			m.XXX_unrecognized = append(m.XXX_unrecognized, dAtA[iNdEx:iNdEx+skippy]...)
			iNdEx += skippy
		}
	}
	if hasFields[0]&uint64(0x00000001) == 0 {
		return new(proto.RequiredNotSetError)
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *Close) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowMysqlxExpect
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Close: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Close: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		default:
			iNdEx = preIndex
			skippy, err := skipMysqlxExpect(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthMysqlxExpect
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			m.XXX_unrecognized = append(m.XXX_unrecognized, dAtA[iNdEx:iNdEx+skippy]...)
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipMysqlxExpect(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowMysqlxExpect
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowMysqlxExpect
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
			return iNdEx, nil
		case 1:
			iNdEx += 8
			return iNdEx, nil
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowMysqlxExpect
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			iNdEx += length
			if length < 0 {
				return 0, ErrInvalidLengthMysqlxExpect
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowMysqlxExpect
					}
					if iNdEx >= l {
						return 0, io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					innerWire |= (uint64(b) & 0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				innerWireType := int(innerWire & 0x7)
				if innerWireType == 4 {
					break
				}
				next, err := skipMysqlxExpect(dAtA[start:])
				if err != nil {
					return 0, err
				}
				iNdEx = start + next
			}
			return iNdEx, nil
		case 4:
			return iNdEx, nil
		case 5:
			iNdEx += 4
			return iNdEx, nil
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
	}
	panic("unreachable")
}

var (
	ErrInvalidLengthMysqlxExpect = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowMysqlxExpect   = fmt.Errorf("proto: integer overflow")
)

func init() {
	proto.RegisterFile("github.com/pingcap/tipb/go-mysqlx/Expect/mysqlx_expect.proto", fileDescriptorMysqlxExpect)
}

var fileDescriptorMysqlxExpect = []byte{
	// 335 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0x52, 0xf4, 0xad, 0x2c, 0x2e,
	0xcc, 0xa9, 0xd0, 0x77, 0xad, 0x28, 0x48, 0x4d, 0x2e, 0xd1, 0xcf, 0x05, 0xf3, 0xe2, 0x53, 0xc1,
	0x3c, 0xbd, 0x82, 0xa2, 0xfc, 0x92, 0x7c, 0x21, 0x5e, 0x88, 0x12, 0x3d, 0x88, 0x12, 0xa5, 0x35,
	0xcc, 0x5c, 0x2c, 0xfe, 0x05, 0xa9, 0x79, 0x42, 0x6e, 0x5c, 0x4c, 0xf9, 0x05, 0x12, 0x8c, 0x0a,
	0x8c, 0x1a, 0x7c, 0x46, 0x0a, 0x7a, 0x28, 0x8a, 0xf4, 0x40, 0x0a, 0xf4, 0x9c, 0x4b, 0x2a, 0xfc,
	0x0b, 0x52, 0x8b, 0x12, 0x4b, 0x32, 0xf3, 0xf3, 0xac, 0x44, 0x5c, 0x23, 0x02, 0x5c, 0x9d, 0x43,
	0xe2, 0x9d, 0x43, 0x22, 0xe2, 0x9d, 0xfd, 0x03, 0x22, 0xe3, 0x03, 0x82, 0x5c, 0xc3, 0x82, 0x98,
	0xf2, 0x0b, 0x84, 0x0c, 0xb9, 0x58, 0x92, 0xf3, 0xf3, 0x52, 0x24, 0x98, 0x14, 0x98, 0x35, 0xb8,
	0x8d, 0x64, 0xb1, 0x9a, 0x94, 0x9f, 0x97, 0x92, 0x09, 0x32, 0x26, 0x08, 0xac, 0x54, 0xea, 0x05,
	0x23, 0x17, 0x27, 0x5c, 0x4c, 0x48, 0x99, 0x8b, 0x37, 0x19, 0xc6, 0x89, 0xcf, 0x4e, 0xad, 0x94,
	0x60, 0x54, 0x60, 0xd2, 0xe0, 0x0d, 0xe2, 0x81, 0x0b, 0x7a, 0xa7, 0x56, 0x0a, 0xa9, 0x73, 0xf1,
	0x23, 0x14, 0x95, 0x25, 0xe6, 0x94, 0xa6, 0x4a, 0x30, 0x29, 0x30, 0x6a, 0xf0, 0x04, 0xf1, 0xc1,
	0x85, 0xc3, 0x40, 0xa2, 0x42, 0xfe, 0x60, 0x6f, 0x31, 0x83, 0xbd, 0x65, 0x80, 0xd7, 0x31, 0x08,
	0x16, 0xc2, 0x9b, 0xbc, 0x50, 0x6f, 0xfa, 0x07, 0xc4, 0x07, 0xbb, 0x86, 0x80, 0xfc, 0xa7, 0x64,
	0xc3, 0x25, 0x84, 0xa9, 0x50, 0x48, 0x90, 0x0b, 0x55, 0xa9, 0x00, 0x83, 0x90, 0x30, 0x17, 0x3f,
	0x42, 0x28, 0xd4, 0x0f, 0x24, 0xc8, 0xa8, 0x64, 0xc7, 0xc5, 0x83, 0x1c, 0x8e, 0x42, 0x12, 0x5c,
	0x58, 0x43, 0x52, 0x80, 0x41, 0x48, 0x84, 0x4b, 0x00, 0x49, 0xc6, 0xd5, 0x37, 0x20, 0x24, 0x52,
	0x80, 0x51, 0x89, 0x9d, 0x8b, 0xd5, 0x39, 0x27, 0xbf, 0x38, 0xd5, 0x49, 0xef, 0xc4, 0x23, 0x39,
	0xc6, 0x0b, 0x8f, 0xe4, 0x18, 0x1f, 0x3c, 0x92, 0x63, 0x9c, 0xf1, 0x58, 0x8e, 0x81, 0x4b, 0x26,
	0x39, 0x3f, 0x57, 0x0f, 0x1c, 0xe3, 0x7a, 0xc9, 0x59, 0x10, 0x46, 0x05, 0x24, 0xce, 0x93, 0x4a,
	0xd3, 0x00, 0x01, 0x00, 0x00, 0xff, 0xff, 0x37, 0x7c, 0x25, 0xf1, 0x1a, 0x02, 0x00, 0x00,
}
