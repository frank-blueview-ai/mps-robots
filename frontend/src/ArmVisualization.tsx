import { useEffect, useRef } from "react";
import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import { STLLoader } from "three/examples/jsm/loaders/STLLoader.js";

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
  visualJointOffsets?: number[];
};

type SceneRefs = {
  renderer: THREE.WebGLRenderer;
  scene: THREE.Scene;
  camera: THREE.PerspectiveCamera;
  controls: OrbitControls;
  animationFrame: number;
  robotRoot: THREE.Group;
  jointGroups: THREE.Group[];
  jointMarkers: THREE.Mesh[];
  jointLabels: THREE.Sprite[];
  jointRings: THREE.Mesh[];
  toolGroup: THREE.Group;
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
const sceneScale = 4.2;

// Lite6 joint origins from UFACTORY/xArm ROS lite6_default_kinematics.yaml.
const lite6JointOrigins = [
  { xyz: [0, 0, 0.2435], rpy: [0, 0, 0] },
  { xyz: [0, 0, 0], rpy: [Math.PI / 2, -Math.PI / 2, Math.PI] },
  { xyz: [0.2002, 0, 0], rpy: [-Math.PI, 0, Math.PI / 2] },
  { xyz: [0.087, -0.22761, 0], rpy: [Math.PI / 2, 0, 0] },
  { xyz: [0, 0, 0], rpy: [Math.PI / 2, 0, 0] },
  { xyz: [0, 0.0625, 0], rpy: [-Math.PI / 2, 0, 0] },
] as const;

const linkNames = ["link_base", "link1", "link2", "link3", "link4", "link5", "link6"] as const;

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

    const camera = new THREE.PerspectiveCamera(50, 1, 0.1, 100);
    camera.position.set(3.8, 2.4, 4.6);

    const controls = new OrbitControls(camera, renderer.domElement);
    controls.enableDamping = true;
    controls.enablePan = false;
    controls.minDistance = 2.8;
    controls.maxDistance = 8;
    controls.maxPolarAngle = Math.PI * 0.5;
    controls.target.set(0.25, 0.45, 0);

    buildLighting(scene);
    buildWorkspace(scene);
    const arm = buildOraArm(scene);

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
    void loadLite6Meshes(refs);

    const resize = () => {
      const rect = host.getBoundingClientRect();
      const width = Math.max(320, rect.width);
      const height = Math.max(320, rect.height);
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
        const renderable = object as THREE.Mesh | THREE.Line | THREE.Sprite;
        if (renderable.geometry) renderable.geometry.dispose();
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
  const jointSummary = (props.status?.xarm_joint_pose || []).slice(0, 6).map((value, index) => `J${index + 1} ${value.toFixed(1)}`).join(" | ") || "J1-J6 waiting";
  const calibrationSummary = (props.visualJointOffsets || []).some((value) => Math.abs(value) > 0.05) ? "Visual home adjusted" : "Raw joint pose";

  return (
    <section className="visualization-band" aria-label="3D arm visualization">
      <div className="visualization-head">
        <div>
          <h3>3D Arm Model</h3>
          <p>ORA-style Lite6 mesh model</p>
        </div>
        <div className="visualization-readouts" aria-label="Lite6 6-joint kinematic chain" data-testid="lite6-chain-readout">
          <span>ORA mesh model</span>
          <span>{calibrationSummary}</span>
          <span>{jointSummary}</span>
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
  scene.add(new THREE.HemisphereLight("#ffffff", "#202b24", 1.35));

  const key = new THREE.DirectionalLight("#ffffff", 3.6);
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

  const fill = new THREE.DirectionalLight("#dcecff", 1.4);
  fill.position.set(-4, 2.5, 4);
  scene.add(fill);

  const rim = new THREE.DirectionalLight("#6aa3ff", 1.2);
  rim.position.set(-5, 3, -4);
  scene.add(rim);
}

function buildWorkspace(scene: THREE.Scene) {
  const floor = new THREE.Mesh(
    new THREE.CircleGeometry(2.1, 96),
    new THREE.MeshStandardMaterial({ color: "#15191d", roughness: 0.84, metalness: 0.08 }),
  );
  floor.rotation.x = -Math.PI / 2;
  floor.receiveShadow = true;
  scene.add(floor);

  const grid = new THREE.GridHelper(4.6, 24, "#39424a", "#22282d");
  grid.position.y = 0.012;
  scene.add(grid);

  const reachRing = new THREE.Mesh(
    new THREE.TorusGeometry(1.85, 0.01, 12, 160),
    new THREE.MeshBasicMaterial({ color: "#58c67a", transparent: true, opacity: 0.62 }),
  );
  reachRing.rotation.x = -Math.PI / 2;
  reachRing.position.y = 0.028;
  scene.add(reachRing);

  const cautionRing = new THREE.Mesh(
    new THREE.TorusGeometry(2.15, 0.012, 12, 160),
    new THREE.MeshBasicMaterial({ color: "#d9a441", transparent: true, opacity: 0.42 }),
  );
  cautionRing.rotation.x = -Math.PI / 2;
  cautionRing.position.y = 0.032;
  scene.add(cautionRing);
}

function buildOraArm(scene: THREE.Scene) {
  const white = new THREE.MeshPhysicalMaterial({
    color: "#f3f4f2",
    roughness: 0.28,
    metalness: 0.04,
    clearcoat: 0.72,
    clearcoatRoughness: 0.22,
  });
  const gray = new THREE.MeshPhysicalMaterial({ color: "#9ea3a7", roughness: 0.34, metalness: 0.42, clearcoat: 0.3 });
  const yellow = new THREE.MeshBasicMaterial({ color: "#ffe500" });
  const black = new THREE.MeshStandardMaterial({ color: "#151719", roughness: 0.55, metalness: 0.18 });
  const red = new THREE.MeshStandardMaterial({ color: "#d6504a", roughness: 0.45, metalness: 0.08 });

  const robotRoot = new THREE.Group();
  robotRoot.name = "ora-lite6-mesh-root";
  robotRoot.rotation.x = -Math.PI / 2;
  robotRoot.scale.setScalar(sceneScale);
  scene.add(robotRoot);

  const jointGroups: THREE.Group[] = [];
  let parent = robotRoot;
  lite6JointOrigins.forEach((origin, index) => {
    const group = new THREE.Group();
    group.name = `ora-joint-${index + 1}`;
    group.position.set(origin.xyz[0], origin.xyz[1], origin.xyz[2]);
    group.rotation.set(origin.rpy[0], origin.rpy[1], origin.rpy[2], "XYZ");
    parent.add(group);
    jointGroups.push(group);
    parent = group;
  });

  const jointMarkers = Array.from({ length: 6 }, (_, index) => {
    const marker = new THREE.Mesh(new THREE.SphereGeometry(0.045, 32, 16), gray);
    marker.name = `ora-joint-marker-${index + 1}`;
    scene.add(marker);
    return marker;
  });

  const jointLabels = Array.from({ length: 6 }, (_, index) => {
    const label = makeLabel(`J${index + 1}`);
    scene.add(label);
    return label;
  });

  const jointBandRadii = [0.075, 0.056, 0.052, 0.044, 0.04, 0.036];
  const jointRings = jointGroups.map((group, index) => {
    const ring = new THREE.Mesh(new THREE.TorusGeometry(jointBandRadii[index], 0.0035, 10, 96), yellow);
    ring.name = `ora-yellow-band-${index + 1}`;
    ring.rotation.x = Math.PI / 2;
    group.add(ring);
    return ring;
  });

  const toolGroup = new THREE.Group();
  toolGroup.name = "ora-tool-group";
  toolGroup.position.set(0.13, 0, 0);
  jointGroups[5].add(toolGroup);

  const toolBody = new THREE.Mesh(new THREE.CylinderGeometry(0.045, 0.055, 0.12, 32), gray);
  toolBody.rotation.z = Math.PI / 2;
  toolBody.castShadow = true;
  toolGroup.add(toolBody);

  const gripperLeft = new THREE.Mesh(new THREE.TorusGeometry(0.035, 0.008, 8, 24, Math.PI * 1.35), black);
  gripperLeft.position.set(0.045, 0.032, 0.035);
  gripperLeft.rotation.set(Math.PI / 2, 0, Math.PI / 8);
  gripperLeft.castShadow = true;
  toolGroup.add(gripperLeft);

  const gripperRight = gripperLeft.clone();
  gripperRight.position.z = -0.035;
  gripperRight.rotation.z = -Math.PI / 8;
  toolGroup.add(gripperRight);

  const tcpMarker = new THREE.Mesh(new THREE.SphereGeometry(0.06, 32, 16), new THREE.MeshStandardMaterial({ color: "#d6a236", roughness: 0.48, metalness: 0.12 }));
  tcpMarker.castShadow = true;
  scene.add(tcpMarker);

  const targetMarker = new THREE.Mesh(
    new THREE.SphereGeometry(0.06, 32, 16),
    new THREE.MeshStandardMaterial({ color: "#7bdcff", emissive: "#104d63", emissiveIntensity: 0.55, roughness: 0.25, metalness: 0.15 }),
  );
  targetMarker.castShadow = true;
  scene.add(targetMarker);

  const targetHalo = new THREE.Mesh(
    new THREE.TorusGeometry(0.14, 0.01, 10, 64),
    new THREE.MeshBasicMaterial({ color: "#7bdcff", transparent: true, opacity: 0.85 }),
  );
  scene.add(targetHalo);

  const intentLine = new THREE.Line(
    new THREE.BufferGeometry().setFromPoints([tcpMarker.position, targetMarker.position]),
    new THREE.LineBasicMaterial({ color: "#7bdcff", transparent: true, opacity: 0.85 }),
  );
  scene.add(intentLine);

  const statusLight = new THREE.PointLight("#58c67a", 1.2, 2.8);
  statusLight.position.set(-1.4, 1.25, 1.4);
  scene.add(statusLight);

  const warningHalo = new THREE.Mesh(new THREE.TorusGeometry(0.52, 0.012, 12, 96), red);
  warningHalo.name = "status-warning-ring";
  warningHalo.rotation.x = -Math.PI / 2;
  warningHalo.position.y = 0.12;
  scene.add(warningHalo);

  return { robotRoot, jointGroups, jointMarkers, jointLabels, jointRings, toolGroup, gripperLeft, gripperRight, tcpMarker, targetMarker, targetHalo, intentLine, statusLight };
}

async function loadLite6Meshes(refs: SceneRefs) {
  const loader = new STLLoader();
  const white = new THREE.MeshPhysicalMaterial({
    color: "#f3f4f2",
    roughness: 0.28,
    metalness: 0.04,
    clearcoat: 0.72,
    clearcoatRoughness: 0.22,
  });
  const gray = new THREE.MeshPhysicalMaterial({ color: "#9ea3a7", roughness: 0.34, metalness: 0.42, clearcoat: 0.3 });

  try {
    const meshes = await Promise.all(linkNames.map(async (name, index) => {
      const geometry = await loader.loadAsync(`/assets/lite6/visual/${name}.stl`);
      geometry.computeVertexNormals();
      const mesh = new THREE.Mesh(geometry, index === 0 || index === 6 ? gray : white);
      mesh.name = `ora-mesh-${name}`;
      mesh.castShadow = true;
      mesh.receiveShadow = true;
      return mesh;
    }));
    refs.robotRoot.add(meshes[0]);
    refs.jointGroups.forEach((group, index) => group.add(meshes[index + 1]));
  } catch (error) {
    console.warn("Lite6 visual meshes could not be loaded", error);
  }
}

function makeLabel(text: string) {
  const canvas = document.createElement("canvas");
  canvas.width = 96;
  canvas.height = 48;
  const context = canvas.getContext("2d");
  if (context) {
    context.fillStyle = "rgba(11, 13, 15, 0.78)";
    context.fillRect(0, 0, canvas.width, canvas.height);
    context.strokeStyle = "#39424a";
    context.strokeRect(1, 1, canvas.width - 2, canvas.height - 2);
    context.fillStyle = "#f4f7f8";
    context.font = "700 22px Segoe UI, Arial";
    context.textAlign = "center";
    context.textBaseline = "middle";
    context.fillText(text, canvas.width / 2, canvas.height / 2);
  }
  const texture = new THREE.CanvasTexture(canvas);
  const sprite = new THREE.Sprite(new THREE.SpriteMaterial({ map: texture, transparent: true }));
  sprite.scale.set(0.3, 0.15, 1);
  return sprite;
}

function updateSceneFromProps(refs: SceneRefs | null, props: ArmVisualizationProps) {
  if (!refs) return;

  refs.controls.enabled = !props.cameraLocked;
  if (props.resetCameraSignal !== refs.resetCameraSignal) {
    refs.resetCameraSignal = props.resetCameraSignal;
    resetCamera(refs);
  }

  const status = props.status;
  const jointAngles = applyVisualJointOffsets(status?.xarm_joint_pose, props.visualJointOffsets);
  const pose = status?.xarm_tcp_pose || [];
  const severity = status?.xarm_error_code ? "error" : props.safeToMove ? "ready" : "blocked";

  refs.jointGroups.forEach((group, index) => {
    group.rotation.set(lite6JointOrigins[index].rpy[0], lite6JointOrigins[index].rpy[1], lite6JointOrigins[index].rpy[2], "XYZ");
    group.rotateZ(degreesToRadians(valueOr(jointAngles[index], 0)));
  });

  refs.jointGroups.forEach((group, index) => {
    group.getWorldPosition(refs.jointMarkers[index].position);
    refs.jointMarkers[index].scale.setScalar(1 + Math.sin(performance.now() / 450 + index) * 0.035);
    refs.jointLabels[index].position.copy(refs.jointMarkers[index].position).add(new THREE.Vector3(0.02, 0.17, 0.02));
  });

  refs.toolGroup.getWorldPosition(refs.tcpMarker.position);

  const telemetryTcp = mapTcpPoseToScene(pose);
  if (pose.length >= 3) {
    refs.tcpMarker.position.lerp(telemetryTcp, 0.12);
  }

  const intent = props.joystickIntent || defaultIntent;
  const intentMagnitude = Math.hypot(intent.x, intent.y, intent.z);
  const target = refs.tcpMarker.position.clone().add(new THREE.Vector3(intent.x * 0.5, intent.z * 0.38, -intent.y * 0.5));
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
  refs.gripperLeft.position.z = THREE.MathUtils.lerp(refs.gripperLeft.position.z, gripperOpen ? 0.085 : 0.035, 0.18);
  refs.gripperRight.position.z = THREE.MathUtils.lerp(refs.gripperRight.position.z, gripperOpen ? -0.085 : -0.035, 0.18);

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
  refs.camera.position.set(3.8, 2.4, 4.6);
  refs.controls.target.set(0.25, 0.45, 0);
  refs.controls.update();
}

function mapTcpPoseToScene(pose: number[]) {
  const x = clamp(valueOr(pose[0], 0) / 220, -1.75, 1.75);
  const y = clamp(valueOr(pose[2], 140) / 170 + 0.18, 0.35, 2.5);
  const z = clamp(valueOr(pose[1], 0) / -220, -1.75, 1.75);
  return new THREE.Vector3(x, y, z);
}

function valueOr(value: number | undefined, fallback: number) {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function applyVisualJointOffsets(jointPose: number[] | undefined, offsets: number[] | undefined) {
  return Array.from({ length: 6 }, (_, index) => valueOr(jointPose?.[index], 0) + valueOr(offsets?.[index], 0));
}

function degreesToRadians(value: number) {
  return (value * Math.PI) / 180;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}
