package vm

import "github.com/joushou/gocnc/gcode"
import "math"
import "fmt"

// Retrieves position from top of stack
func (vm *Machine) curPos() Position {
	return vm.Positions[len(vm.Positions)-1]
}

// Appends a position to the stack
func (vm *Machine) addPos(pos Position) {
	vm.Positions = append(vm.Positions, pos)
}

// Calculates the absolute position of the given statement, including optional I, J, K parameters
func (vm *Machine) calcPos(stmt Statement) (newX, newY, newZ, newI, newJ, newK float64) {
	pos := vm.curPos()
	var err error

	if newX, err = stmt.get('X'); err != nil {
		newX = pos.x
	} else if !vm.metric {
		newX *= 25.4
	}

	if newY, err = stmt.get('Y'); err != nil {
		newY = pos.y
	} else if !vm.metric {
		newY *= 25.4
	}

	if newZ, err = stmt.get('Z'); err != nil {
		newZ = pos.z
	} else if !vm.metric {
		newZ *= 25.4
	}

	newI = stmt.getDefault('I', 0)
	newJ = stmt.getDefault('J', 0)
	newK = stmt.getDefault('K', 0)

	if !vm.metric {
		newI, newJ, newK = newI*25.4, newJ*25.4, newZ*25.4
	}

	if !vm.absoluteMove {
		newX, newY, newZ = pos.x+newX, pos.y+newY, pos.z+newZ
	}

	if !vm.absoluteArc {
		newI, newJ, newK = pos.x+newI, pos.y+newJ, pos.z+newK
	}
	return newX, newY, newZ, newI, newJ, newK
}

// Adds a simple linear move
func (vm *Machine) positioning(stmt Statement) {
	newX, newY, newZ, _, _, _ := vm.calcPos(stmt)
	vm.addPos(Position{vm.state, newX, newY, newZ})
}

// Calculates an approximate arc from the provided statement
func (vm *Machine) approximateArc(stmt Statement) {
	var (
		startPos                           Position = vm.curPos()
		endX, endY, endZ, endI, endJ, endK float64  = vm.calcPos(stmt)
		s1, s2, s3, e1, e2, e3, c1, c2     float64
		add                                func(x, y, z float64)
		clockwise                          bool = (vm.state.moveMode == moveModeCWArc)
	)

	vm.state.moveMode = moveModeLinear

	// Read the additional rotation parameter
	P := 0.0
	if pp, err := stmt.get('P'); err == nil {
		P = pp
	}

	//  Flip coordinate system for working in other planes
	switch vm.movePlane {
	case planeXY:
		s1, s2, s3, e1, e2, e3, c1, c2 = startPos.x, startPos.y, startPos.z, endX, endY, endZ, endI, endJ
		add = func(x, y, z float64) {
			wx, wy, wz := gcode.Word{'X', x}, gcode.Word{'Y', y}, gcode.Word{'Z', z}
			vm.positioning(Statement{&wx, &wy, &wz})
		}
	case planeXZ:
		s1, s2, s3, e1, e2, e3, c1, c2 = startPos.z, startPos.x, startPos.y, endZ, endX, endY, endK, endI
		add = func(x, y, z float64) {
			wx, wy, wz := gcode.Word{'X', y}, gcode.Word{'Y', z}, gcode.Word{'Z', x}
			vm.positioning(Statement{&wx, &wy, &wz})

		}
	case planeYZ:
		s1, s2, s3, e1, e2, e3, c1, c2 = startPos.y, startPos.z, startPos.x, endY, endZ, endX, endJ, endK
		add = func(x, y, z float64) {
			wx, wy, wz := gcode.Word{'X', z}, gcode.Word{'Y', x}, gcode.Word{'Z', y}
			vm.positioning(Statement{&wx, &wy, &wz})
		}
	}

	radius1 := math.Sqrt(math.Pow(c1-s1, 2) + math.Pow(c2-s2, 2))
	radius2 := math.Sqrt(math.Pow(c1-e1, 2) + math.Pow(c2-e2, 2))
	if radius1 == 0 || radius2 == 0 {
		panic("Invalid arc statement")
	}

	if math.Abs((radius2-radius1)/radius1) > 0.01 {
		panic(fmt.Sprintf("Radius deviation of %f percent", math.Abs((radius2-radius1)/radius1)*100))
	}

	theta1 := math.Atan2((s2 - c2), (s1 - c1))
	theta2 := math.Atan2((e2 - c2), (e1 - c1))

	angleDiff := theta2 - theta1
	if angleDiff < 0 && !clockwise {
		angleDiff += 2 * math.Pi
	} else if angleDiff > 0 && clockwise {
		angleDiff -= 2 * math.Pi
	}

	if clockwise {
		angleDiff -= P * 2 * math.Pi
	} else {
		angleDiff += P * 2 * math.Pi
	}

	steps := 1
	if vm.MaxArcDeviation < radius1 {
		steps = int(math.Ceil(math.Abs(angleDiff / (2 * math.Acos(1-vm.MaxArcDeviation/radius1)))))
	}

	// Enforce a minimum line length
	arcLen := math.Abs(angleDiff) * math.Sqrt(math.Pow(radius1, 2)+math.Pow((e3-s3)/angleDiff, 2))
	steps2 := int(arcLen / vm.MinArcLineLength)

	if steps > steps2 {
		steps = steps2
	}

	angle := 0.0
	for i := 0; i <= steps; i++ {
		angle = theta1 + angleDiff/float64(steps)*float64(i)
		a1, a2 := c1+radius1*math.Cos(angle), c2+radius1*math.Sin(angle)
		a3 := s3 + (e3-s3)/float64(steps)*float64(i)
		add(a1, a2, a3)
	}
	add(e1, e2, e3)
}
