import { useEffect, useRef } from "react";
import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";

type RobotStatus3D = {
  xarm_connected?: boolean;
  xarm_is_ready?: boolean;
  xarm_state?: number;
  xarm_error_code?: number;
  xarm_tcp_pose?: number[];
  xarm_joint_pose?: number[];
};

export type JoystickIntent = {
  x: number;
  y: number;
  z: number;
  label: string;
};

type ArmVisualizationProps = {
  status?: RobotStatus3D;
  blocked: string;
  liveControl: boolean;
  safeToMove: boolean;
  joystickIntent: JoystickIntent;
  followPose: boolean;
  cameraLocked: boolean;
  resetCameraSignal: number;
};

type SceneRefs = {
  renderer: THREE.WebGLRenderer;
  scene: THREE.Scene;
  camera: THREE.PerspectiveCamera;
  controls: OrbitControls;
  animationFrame: number;
  basePivot: THREE.Group;
  shoulderPivot: THREE.Group;
  elbowPivot: THREE.Group;
  wristPivot: THREE.Group;
  gripperLeft: THREE.Mesh;
  gripperRight: THREE.Mesh;
  tcpMarker: THREE.Mesh;
  targetMarker: THREE.Mesh;
  targetHalo: THREE.Mesh;
  intentLine: THREE.Line;
  statusLight: THREE.PointLight;
  resetCameraSignal: number;
};

const defaultIntent: JoystickIntent = { x: 0, y: 0, z: 0, label: "idle" };

export function ArmVisualization(props: ArmVisualizationProps) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const sceneRef = useRef<SceneRefs | null>(null);
  const frozenStatusRef = useRef<RobotStatus3D | undefined>(props.status);
  const propsRef = useRef({ ...props, joystickIntent: props.joystickIntent || defaultIntent });

  useEffect(() => {
    if (props.followPose) {
      frozenStatusRef.current = props.status;
    }

    propsRef.current = {
      ...props,
      status: props.followPose ? props.status : frozenStatusRef.current,
      joystickIntent: props.joystickIntent || defaultIntent,
    };
    updateSceneFromProps(sceneRef.current, propsRef.current);
  }, [props]);

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    const renderer = new THREE.WebGLRenderer({ antialias: true, preserveDrawingBuffer: true });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    renderer.shadowMap.enabled = true;
    renderer.shadowMap.type = THREE.PCFSoftShadowMap;
    renderer.domElement.dataset.testid = "arm-3d-canvas";
    host.appendChild(renderer.domElement);

    const scene = new THREE.Scene();
    scene.background = new THREE.Color("#0b0d0f");
    scene.fog = new THREE.Fog("#0b0d0f", 7, 16);

    const camera = new THREE.PerspectiveCamera(42, 1, 0.1, 100);
    camera.position.set(4.3, 3.2, 5.2);

    const controls = new OrbitControls(camera, renderer.domElement);
    controls.enableDamping = true;
    controls.enablePan = false;
    controls.minDistance = 3.8;
    controls.maxDistance = 9;
    controls.maxPolarAngle = Math.PI * 0.48;
    controls.target.set(0, 1.3, 0);

    buildLighting(scene);
    buildWorkspace(scene);
    const arm = buildArm(scene);

    const refs: SceneRefs = {
      renderer,
      scene,
      camera,
      controls,
      animationFrame: 0,
      resetCameraSignal: propsRef.current.resetCameraSignal,
      ...arm,
    };
    sceneRef.current = refs;

    const resize = () => {
      const rect = host.getBoundingClientRect();
      const width = Math.max(320, rect.width);
      const height = Math.max(200, rect.height);
      renderer.setSize(width, height, false);
      camera.aspect = width / height;
      camera.updateProjectionMatrix();
    };

    const animate = () => {
      updateSceneFromProps(refs, propsRef.current);
      controls.update();
      renderer.render(scene, camera);
      refs.animationFrame = window.requestAnimationFrame(animate);
    };

    resize();
    resetCamera(refs);
    const resizeObserver = new ResizeObserver(resize);
    resizeObserver.observe(host);
    refs.animationFrame = window.requestAnimationFrame(animate);

    return () => {
      resizeObserver.disconnect();
      window.cancelAnimationFrame(refs.animationFrame);
      controls.dispose();
      scene.traverse((object) => {
        const renderable = object as THREE.Mesh | THREE.Line;
        if (renderable.geometry) {
          renderable.geometry.dispose();
        }
        if (renderable.material) {
          const materials = Array.isArray(renderable.material) ? renderable.material : [renderable.material];
          materials.forEach((material) => material.dispose());
        }
      });
      renderer.dispose();
      renderer.domElement.remove();
      sceneRef.current = null;
    };
  }, []);

  const statusLabel = props.safeToMove ? "Ready" : props.blocked || "Blocked";
  const intentLabel = props.joystickIntent.label || "idle";

  return (
    <section className="visualization-band" aria-label="3D arm visualization">
      <div className="visualization-head">
        <div>
          <h3>3D Arm Model</h3>
          <p>Telemetry-driven arm pose and joystick intent</p>
        </div>
        <div className="visualization-readouts">
          <span>{props.liveControl ? "Live control armed" : "Live control locked"}</span>
          <span>{props.followPose ? "Following live pose" : "Model frozen"}</span>
          <span>{props.cameraLocked ? "Camera locked" : "Camera unlocked"}</span>
          <span>{statusLabel}</span>
          <span>Intent: {intentLabel}</span>
        </div>
      </div>
      <div className="three-host" ref={hostRef} />
    </section>
  );
}

function buildLighting(scene: THREE.Scene) {
  scene.add(new THREE.HemisphereLight("#dcecff", "#202b24", 1.1));

  const key = new THREE.DirectionalLight("#ffffff", 3.2);
  key.position.set(4, 6, 5);
  key.castShadow = true;
  key.shadow.mapSize.set(2048, 2048);
  key.shadow.camera.near = 0.1;
  key.shadow.camera.far = 16;
  key.shadow.camera.left = -5;
  key.shadow.camera.right = 5;
  key.shadow.camera.top = 5;
  key.shadow.camera.bottom = -5;
  scene.add(key);

  const rim = new THREE.DirectionalLight("#6aa3ff", 1.4);
  rim.position.set(-5, 3, -4);
  scene.add(rim);
}

function buildWorkspace(scene: THREE.Scene) {
  const floor = new THREE.Mesh(
    new THREE.CircleGeometry(3.15, 96),
    new THREE.MeshStandardMaterial({ color: "#15191d", roughness: 0.84, metalness: 0.08 }),
  );
  floor.rotation.x = -Math.PI / 2;
  floor.receiveShadow = true;
  scene.add(floor);

  const grid = new THREE.GridHelper(6.2, 24, "#39424a", "#22282d");
  grid.position.y = 0.012;
  scene.add(grid);

  const reachRing = new THREE.Mesh(
    new THREE.TorusGeometry(2.25, 0.01, 12, 160),
    new THREE.MeshBasicMaterial({ color: "#58c67a", transparent: true, opacity: 0.62 }),
  );
  reachRing.rotation.x = -Math.PI / 2;
  reachRing.position.y = 0.028;
  scene.add(reachRing);

  const cautionRing = new THREE.Mesh(
    new THREE.TorusGeometry(2.88, 0.012, 12, 160),
    new THREE.MeshBasicMaterial({ color: "#d9a441", transparent: true, opacity: 0.42 }),
  );
  cautionRing.rotation.x = -Math.PI / 2;
  cautionRing.position.y = 0.032;
  scene.add(cautionRing);

  const table = new THREE.Mesh(
    new THREE.CylinderGeometry(0.62, 0.72, 0.18, 64),
    new THREE.MeshStandardMaterial({ color: "#293136", roughness: 0.7, metalness: 0.35 }),
  );
  table.position.y = 0.09;
  table.castShadow = true;
  table.receiveShadow = true;
  scene.add(table);
}

function buildArm(scene: THREE.Scene) {
  const green = new THREE.MeshStandardMaterial({ color: "#4e8f3a", roughness: 0.46, metalness: 0.18 });
  const dark = new THREE.MeshStandardMaterial({ color: "#1d2428", roughness: 0.58, metalness: 0.32 });
  const metal = new THREE.MeshStandardMaterial({ color: "#bac4c9", roughness: 0.32, metalness: 0.7 });
  const orange = new THREE.MeshStandardMaterial({ color: "#d6a236", roughness: 0.48, metalness: 0.12 });
  const red = new THREE.MeshStandardMaterial({ color: "#d6504a", roughness: 0.45, metalness: 0.08 });

  const basePivot = new THREE.Group();
  scene.add(basePivot);

  const base = new THREE.Mesh(new THREE.CylinderGeometry(0.46, 0.54, 0.28, 72), green);
  base.position.y = 0.24;
  base.castShadow = true;
  base.receiveShadow = true;
  basePivot.add(base);

  const shoulderPivot = new THREE.Group();
  shoulderPivot.position.set(0, 0.45, 0);
  basePivot.add(shoulderPivot);

  const shoulder = jointSphere(0.28, green);
  shoulderPivot.add(shoulder);

  const upperLink = roundedLink(0.2, 1.18, dark);
  upperLink.position.y = 0.58;
  shoulderPivot.add(upperLink);

  const elbowPivot = new THREE.Group();
  elbowPivot.position.set(0, 1.16, 0);
  shoulderPivot.add(elbowPivot);

  const elbow = jointSphere(0.23, green);
  elbowPivot.add(elbow);

  const forearm = roundedLink(0.17, 0.98, metal);
  forearm.position.y = 0.48;
  elbowPivot.add(forearm);

  const wristPivot = new THREE.Group();
  wristPivot.position.set(0, 0.97, 0);
  elbowPivot.add(wristPivot);

  const wrist = jointSphere(0.18, green);
  wristPivot.add(wrist);

  const wristLink = roundedLink(0.12, 0.5, dark);
  wristLink.position.y = 0.25;
  wristPivot.add(wristLink);

  const tool = new THREE.Group();
  tool.position.set(0, 0.52, 0);
  wristPivot.add(tool);

  const toolBody = new THREE.Mesh(new THREE.BoxGeometry(0.42, 0.16, 0.26), orange);
  toolBody.castShadow = true;
  tool.add(toolBody);

  const gripperLeft = new THREE.Mesh(new THREE.BoxGeometry(0.08, 0.42, 0.08), metal);
  gripperLeft.position.set(-0.16, -0.18, 0);
  gripperLeft.castShadow = true;
  tool.add(gripperLeft);

  const gripperRight = new THREE.Mesh(new THREE.BoxGeometry(0.08, 0.42, 0.08), metal);
  gripperRight.position.set(0.16, -0.18, 0);
  gripperRight.castShadow = true;
  tool.add(gripperRight);

  const tcpMarker = new THREE.Mesh(new THREE.SphereGeometry(0.095, 32, 16), orange);
  tcpMarker.position.set(0, 1.75, 0);
  tcpMarker.castShadow = true;
  scene.add(tcpMarker);

  const targetMarker = new THREE.Mesh(
    new THREE.SphereGeometry(0.085, 32, 16),
    new THREE.MeshStandardMaterial({ color: "#7bdcff", emissive: "#104d63", emissiveIntensity: 0.55, roughness: 0.25, metalness: 0.15 }),
  );
  targetMarker.position.set(0, 1.75, 0);
  targetMarker.castShadow = true;
  scene.add(targetMarker);

  const targetHalo = new THREE.Mesh(
    new THREE.TorusGeometry(0.18, 0.01, 10, 64),
    new THREE.MeshBasicMaterial({ color: "#7bdcff", transparent: true, opacity: 0.85 }),
  );
  targetHalo.position.copy(targetMarker.position);
  scene.add(targetHalo);

  const intentLine = new THREE.Line(
    new THREE.BufferGeometry().setFromPoints([tcpMarker.position, targetMarker.position]),
    new THREE.LineBasicMaterial({ color: "#7bdcff", transparent: true, opacity: 0.85 }),
  );
  scene.add(intentLine);

  const statusLight = new THREE.PointLight("#58c67a", 1.2, 2.8);
  statusLight.position.set(-1.4, 1.25, 1.4);
  scene.add(statusLight);

  const warningHalo = new THREE.Mesh(
    new THREE.TorusGeometry(0.68, 0.012, 12, 96),
    red,
  );
  warningHalo.name = "status-warning-ring";
  warningHalo.rotation.x = -Math.PI / 2;
  warningHalo.position.y = 0.12;
  scene.add(warningHalo);

  return {
    basePivot,
    shoulderPivot,
    elbowPivot,
    wristPivot,
    gripperLeft,
    gripperRight,
    tcpMarker,
    targetMarker,
    targetHalo,
    intentLine,
    statusLight,
  };
}

function jointSphere(radius: number, material: THREE.Material) {
  const mesh = new THREE.Mesh(new THREE.SphereGeometry(radius, 48, 24), material);
  mesh.castShadow = true;
  mesh.receiveShadow = true;
  return mesh;
}

function roundedLink(radius: number, length: number, material: THREE.Material) {
  const group = new THREE.Group();

  const body = new THREE.Mesh(new THREE.CylinderGeometry(radius, radius, length, 36), material);
  body.castShadow = true;
  body.receiveShadow = true;
  group.add(body);

  const capTop = new THREE.Mesh(new THREE.SphereGeometry(radius, 36, 16), material);
  capTop.position.y = length / 2;
  capTop.castShadow = true;
  group.add(capTop);

  const capBottom = new THREE.Mesh(new THREE.SphereGeometry(radius, 36, 16), material);
  capBottom.position.y = -length / 2;
  capBottom.castShadow = true;
  group.add(capBottom);

  return group;
}

function updateSceneFromProps(refs: SceneRefs | null, props: ArmVisualizationProps) {
  if (!refs) return;

  refs.controls.enabled = !props.cameraLocked;
  if (props.resetCameraSignal !== refs.resetCameraSignal) {
    refs.resetCameraSignal = props.resetCameraSignal;
    resetCamera(refs);
  }

  const status = props.status;
  const joints = status?.xarm_joint_pose || [];
  const pose = status?.xarm_tcp_pose || [];
  const severity = status?.xarm_error_code ? "error" : props.safeToMove ? "ready" : "blocked";

  refs.basePivot.rotation.y = degreesToRadians(valueOr(joints[0], 20));
  refs.shoulderPivot.rotation.z = degreesToRadians(-34 + valueOr(joints[1], 18) * 0.45);
  refs.elbowPivot.rotation.z = degreesToRadians(54 - valueOr(joints[2], 34) * 0.45);
  refs.wristPivot.rotation.z = degreesToRadians(-18 + valueOr(joints[3], 8) * 0.35);
  refs.wristPivot.rotation.y = degreesToRadians(valueOr(joints[4], 0) * 0.25);

  const tcp = mapTcpPoseToScene(pose);
  refs.tcpMarker.position.lerp(tcp, 0.18);

  const intent = props.joystickIntent || defaultIntent;
  const intentMagnitude = Math.hypot(intent.x, intent.y, intent.z);
  const target = tcp.clone().add(new THREE.Vector3(intent.x * 0.62, intent.z * 0.46, -intent.y * 0.62));
  refs.targetMarker.position.lerp(target, 0.2);
  refs.targetMarker.visible = intentMagnitude > 0.05 || props.liveControl;
  refs.targetHalo.visible = refs.targetMarker.visible;
  refs.targetHalo.position.copy(refs.targetMarker.position);
  refs.targetHalo.rotation.x = Math.PI / 2;
  refs.targetHalo.rotation.z += 0.018;

  const linePositions = refs.intentLine.geometry.attributes.position as THREE.BufferAttribute;
  linePositions.setXYZ(0, refs.tcpMarker.position.x, refs.tcpMarker.position.y, refs.tcpMarker.position.z);
  linePositions.setXYZ(1, refs.targetMarker.position.x, refs.targetMarker.position.y, refs.targetMarker.position.z);
  linePositions.needsUpdate = true;
  refs.intentLine.visible = refs.targetMarker.visible && refs.tcpMarker.position.distanceTo(refs.targetMarker.position) > 0.04;

  const gripperOpen = props.liveControl && props.safeToMove;
  refs.gripperLeft.position.x = THREE.MathUtils.lerp(refs.gripperLeft.position.x, gripperOpen ? -0.2 : -0.12, 0.18);
  refs.gripperRight.position.x = THREE.MathUtils.lerp(refs.gripperRight.position.x, gripperOpen ? 0.2 : 0.12, 0.18);

  const lightColor = severity === "ready" ? "#58c67a" : severity === "blocked" ? "#d9a441" : "#d6504a";
  refs.statusLight.color.set(lightColor);
  refs.statusLight.intensity = severity === "ready" ? 1.2 : 2.1;

  const warningRing = refs.scene.getObjectByName("status-warning-ring");
  if (warningRing) {
    warningRing.visible = severity !== "ready";
    warningRing.rotation.z += severity === "error" ? 0.025 : 0.01;
  }

}

function resetCamera(refs: SceneRefs) {
  refs.camera.position.set(4.3, 3.2, 5.2);
  refs.controls.target.set(0, 1.3, 0);
  refs.controls.update();
}

function mapTcpPoseToScene(pose: number[]) {
  const x = clamp(valueOr(pose[0], 0) / 160, -1.8, 1.8);
  const y = clamp(valueOr(pose[2], 140) / 150 + 0.34, 0.45, 2.7);
  const z = clamp(valueOr(pose[1], 0) / 160, -1.8, 1.8);
  return new THREE.Vector3(x, y, z);
}

function valueOr(value: number | undefined, fallback: number) {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function degreesToRadians(value: number) {
  return (value * Math.PI) / 180;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}
