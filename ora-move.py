import enum
import time

try:
    enum.StrEnum
except AttributeError:
    from strenum import StrEnum

    enum.StrEnum = StrEnum

from ozobot.ora.simple import Cartesian, FingerGripperState, gripper, move


def move_to(x: float, y: float, z: float) -> None:
    move.linear(Cartesian(x, y, z))


def main():
    print("Starting Ozobot ORA Arm sequence...")

    # 1. Move to a safe starting position (Home)
    # Coordinates are in millimeters: x, y, z
    print("Moving to starting position...")
    move_to(150, 0, 100)
    
    # 2. Open the gripper
    print("Opening gripper...")
    gripper.set_state(FingerGripperState.OPEN)
    time.sleep(1) # Wait for action to complete
    
    # 3. Move down to pick up an object
    print("Moving down to target...")
    move_to(200, 0, 20)
    
    # 4. Close the gripper
    print("Closing gripper...")
    gripper.set_state(FingerGripperState.CLOSED)
    time.sleep(1)
    
    # 5. Lift the object
    print("Lifting object...")
    move_to(200, 0, 100)
    
    # 6. Move to a new location
    print("Moving to drop-off location...")
    move_to(0, 200, 100)
    
    # 7. Lower and release
    print("Lowering...")
    move_to(0, 200, 20)
    print("Releasing gripper...")
    gripper.set_state(FingerGripperState.OPEN)
    
    # 8. Return to home
    print("Returning home...")
    move_to(150, 0, 100)
    
    print("Sequence complete.")

if __name__ == "__main__":
    main()
