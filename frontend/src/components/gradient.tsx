"use client";

import { useEffect, useRef, useState } from "react";

/**
 * The phoenix gradient, drifting.
 *
 * DESIGN.md sanctions exactly one gradient — orange through cream to violet,
 * behind the hero, with "subtle scale/opacity animation" — and forbids colour
 * as decoration everywhere else. So this is the only animated surface in the
 * product, and it does not appear on any application screen.
 *
 * Raw WebGL rather than a scene library: this is two triangles and a fragment
 * shader. three.js would be a megabyte of scene graph, camera, and renderer to
 * draw a rectangle.
 */

const VERTEX = `
attribute vec2 position;
void main() { gl_Position = vec4(position, 0.0, 1.0); }
`;

/*
 * Three radial fields drifting on offset sine paths, composited over cream.
 * The colours are the DESIGN.md gradient stops; the movement is slow enough to
 * read as atmosphere rather than animation.
 */
const FRAGMENT = `
precision mediump float;
uniform vec2 uResolution;
uniform float uTime;
uniform vec2 uPointer;
uniform float uPointerStrength;

vec3 field(vec2 uv, vec2 centre, vec3 colour, float radius) {
  float d = distance(uv, centre);
  return colour * smoothstep(radius, 0.0, d);
}

void main() {
  vec2 uv = gl_FragCoord.xy / uResolution.xy;
  // Correct for aspect so the fields stay circular on a wide viewport.
  uv.x *= uResolution.x / uResolution.y;
  float aspect = uResolution.x / uResolution.y;

  float t = uTime * 0.06;

  vec3 orange = vec3(0.910, 0.251, 0.051);
  vec3 cream  = vec3(1.000, 0.933, 0.847);
  vec3 violet = vec3(0.816, 0.698, 1.000);

  vec3 colour = cream;
  colour = mix(colour, orange, 0.85 * smoothstep(0.95, 0.0,
    distance(uv, vec2(aspect * (0.08 + 0.05 * sin(t)), 1.05 + 0.04 * cos(t * 0.8)))));
  colour = mix(colour, violet, 0.80 * smoothstep(1.05, 0.0,
    distance(uv, vec2(aspect * (0.88 + 0.06 * cos(t * 0.9)), -0.05 + 0.05 * sin(t * 1.1)))));
  colour = mix(colour, cream, 0.55 * smoothstep(0.75, 0.0,
    distance(uv, vec2(aspect * (0.5 + 0.08 * sin(t * 0.7)), 0.5 + 0.06 * cos(t)))));

  // A restrained pool of light follows the pointer. It gives the hero depth
  // without turning the application itself into a GPU-heavy scene.
  float pointerField = smoothstep(0.52, 0.0,
    distance(uv, vec2(aspect * uPointer.x, uPointer.y)));
  colour = mix(colour, vec3(1.0), pointerField * uPointerStrength * 0.13);

  // A little noise breaks the banding a smooth gradient shows on 8-bit panels.
  float grain = fract(sin(dot(gl_FragCoord.xy, vec2(12.9898, 78.233))) * 43758.5453);
  colour += (grain - 0.5) * 0.012;

  gl_FragColor = vec4(colour, 1.0);
}
`;

function compile(gl: WebGLRenderingContext, kind: number, source: string) {
  const shader = gl.createShader(kind);
  if (!shader) return null;
  gl.shaderSource(shader, source);
  gl.compileShader(shader);
  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
    gl.deleteShader(shader);
    return null;
  }
  return shader;
}

export function PhoenixGradient({ className }: { className?: string }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  // Until WebGL has drawn a frame, the CSS gradient underneath is what shows —
  // so a machine without WebGL, or one that fails to compile, still gets the
  // right picture rather than a white hole.
  const [live, setLive] = useState(false);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    // preserveDrawingBuffer keeps the frame readable after compositing. Without
    // it the buffer is discarded, and any screenshot or canvas readback of this
    // page comes back empty even though it renders correctly on screen — which
    // is how it first appeared to be broken.
    const gl = canvas.getContext("webgl", {
      antialias: false,
      alpha: false,
      preserveDrawingBuffer: true,
    });
    if (!gl) return;

    const vertex = compile(gl, gl.VERTEX_SHADER, VERTEX);
    const fragment = compile(gl, gl.FRAGMENT_SHADER, FRAGMENT);
    const program = gl.createProgram();
    if (!vertex || !fragment || !program) return;

    gl.attachShader(program, vertex);
    gl.attachShader(program, fragment);
    gl.linkProgram(program);
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) return;
    gl.useProgram(program);

    const buffer = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
    // Two triangles covering clip space. There is no geometry beyond this.
    gl.bufferData(
      gl.ARRAY_BUFFER,
      new Float32Array([-1, -1, 3, -1, -1, 3]),
      gl.STATIC_DRAW,
    );
    const position = gl.getAttribLocation(program, "position");
    gl.enableVertexAttribArray(position);
    gl.vertexAttribPointer(position, 2, gl.FLOAT, false, 0, 0);

    const uResolution = gl.getUniformLocation(program, "uResolution");
    const uTime = gl.getUniformLocation(program, "uTime");
    const uPointer = gl.getUniformLocation(program, "uPointer");
    const uPointerStrength = gl.getUniformLocation(program, "uPointerStrength");

    // Half resolution: this is an out-of-focus gradient, and rendering it at
    // device pixel ratio on a 4K display costs four times the fill rate for
    // something nobody can see the edges of.
    const resize = () => {
      const { clientWidth, clientHeight } = canvas;
      canvas.width = Math.max(1, Math.floor(clientWidth / 2));
      canvas.height = Math.max(1, Math.floor(clientHeight / 2));
      gl.viewport(0, 0, canvas.width, canvas.height);
      gl.uniform2f(uResolution, canvas.width, canvas.height);
    };
    resize();

    const observer = new ResizeObserver(resize);
    observer.observe(canvas);

    const still = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const started = performance.now();
    let frame = 0;
    let pointerX = 0.5;
    let pointerY = 0.5;
    let pointerStrength = 0;
    let targetX = pointerX;
    let targetY = pointerY;
    let targetStrength = 0;

    const point = (event: PointerEvent) => {
      const bounds = canvas.getBoundingClientRect();
      const inside = event.clientX >= bounds.left && event.clientX <= bounds.right
        && event.clientY >= bounds.top && event.clientY <= bounds.bottom;
      if (!inside) {
        targetStrength = 0;
        return;
      }
      targetX = (event.clientX - bounds.left) / bounds.width;
      targetY = 1 - (event.clientY - bounds.top) / bounds.height;
      targetStrength = 1;
    };
    if (!still) window.addEventListener("pointermove", point, { passive: true });

    const draw = () => {
      // Reduced motion gets the composition, held still. The gradient is the
      // design; the drift is the flourish, and only the flourish is dropped.
      gl.uniform1f(uTime, still ? 0 : (performance.now() - started) / 1000);
      pointerX += (targetX - pointerX) * 0.06;
      pointerY += (targetY - pointerY) * 0.06;
      pointerStrength += (targetStrength - pointerStrength) * 0.05;
      gl.uniform2f(uPointer, pointerX, pointerY);
      gl.uniform1f(uPointerStrength, pointerStrength);
      gl.drawArrays(gl.TRIANGLES, 0, 3);
      if (!still) frame = requestAnimationFrame(draw);
    };
    draw();
    setLive(true);

    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
      window.removeEventListener("pointermove", point);
      // Deliberately no loseContext() here.
      //
      // It looks like the careful thing to do and it permanently breaks this
      // component. React runs mount -> cleanup -> mount in development, and a
      // lost context stays lost: getContext hands the second run the same dead
      // context back, no program binds, and nothing ever draws again. The same
      // happens on any remount, so navigating away and back would kill it in
      // production too. The browser reclaims the context when the canvas is
      // collected, which is all that was needed.
    };
  }, []);

  return (
    <div
      aria-hidden
      className={className}
      style={{ background: "var(--gradient-phoenix)" }}
    >
      <canvas
        ref={canvasRef}
        className="size-full transition-opacity duration-700"
        style={{ opacity: live ? 1 : 0 }}
      />
    </div>
  );
}
