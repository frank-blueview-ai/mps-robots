package robotsim

type LinkGeometry struct {
	Name   string  `json:"name"`
	Length float64 `json:"length"` // in mm
}

type RobotModel struct {
	BaseHeight float64        `json:"base_height"`
	Links      []LinkGeometry `json:"links"`
}

var DefaultRobotModel = RobotModel{
	BaseHeight: 80.0,
	Links: []LinkGeometry{
		{Name: "link1", Length: 120.0},
		{Name: "link2", Length: 100.0},
		{Name: "link3", Length: 80.0},
	},
}
