package main

import (
	"bufio"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/ncruces/zenity"
)

const (
	width            = 1280
	height           = 720
	baseSpeed        = 100.0
	mouseSensitivity = 0.1
)

var (
	cameraPos   = mgl32.Vec3{0, 0, 0}
	cameraFront = mgl32.Vec3{0, 0, -1}
	cameraUp    = mgl32.Vec3{0, 1, 0}
	yaw         = -90.0
	pitch       = 0.0
	lastX       = float64(width / 2)
	lastY       = float64(height / 2)
	firstMouse  = true
	deltaTime   = 0.0
	lastFrame   = 0.0

	// Speed multiplier variables
	speedMultipliers  = []float64{1, 5, 10, 20, 40}
	currentSpeedIndex = 0
	lastShiftState    = glfw.Release
	
	// Directory tracking
	lastDirectory string
)

const (
	vertexShaderSource = `
    #version 410
    layout (location = 0) in vec3 position;
    
    uniform mat4 projection;
    uniform mat4 camera;
    uniform mat4 model;
    
    out vec3 FragPos;
    
    void main() {
        FragPos = vec3(model * vec4(position, 1.0));
        gl_Position = projection * camera * model * vec4(position, 1.0);
    }
    ` + "\x00"

	fragmentShaderSource = `
    #version 410
    in vec3 FragPos;
    uniform bool isWireframe;
    
    out vec4 color;
    
    void main() {
        if (isWireframe) {
            color = vec4(0.0, 0.0, 0.0, 1.0); // Black wireframe
        } else {
            vec3 lightPos = vec3(2000.0, 1000.0, 2000.0);
            vec3 lightColor = vec3(1.0, 1.0, 1.0);
            vec3 objectColor = vec3(0.5, 0.7, 1.0); // Light blue color
            
            // Ambient
            float ambientStrength = 0.3;
            vec3 ambient = ambientStrength * lightColor;
            
            // Diffuse
            vec3 lightDir = normalize(lightPos - FragPos);
            float diff = max(dot(normalize(vec3(0.0, 1.0, 0.0)), lightDir), 0.0);
            vec3 diffuse = diff * lightColor;
            
            vec3 result = (ambient + diffuse) * objectColor;
            color = vec4(result, 1.0);
        }
    }
    ` + "\x00"
)

type Bounds struct {
	minX, minY, minZ float32
	maxX, maxY, maxZ float32
}

type MeshData struct {
	vertices []float32
	indices  []uint32
	bounds   Bounds
	vao      uint32
	vbo      uint32
	ebo      uint32
}

type Scene struct {
    meshes []MeshData
    bounds Bounds
}

var scene Scene

func init() {
	runtime.LockOSThread()
}

func main() {
    if err := glfw.Init(); err != nil {
        log.Fatal(err)
    }
    defer glfw.Terminate()

    filenames, err := selectOBJFiles("")
    if err != nil {
        log.Fatal("Initial file selection failed:", err)
    }

    window, program := initializeWindow(strings.Join(filenames, ", "))
    defer window.Destroy()

    scene = loadAllMeshes(program, filenames)
    
    for !window.ShouldClose() {
        currentFrame := glfw.GetTime()
        deltaTime = currentFrame - lastFrame
        lastFrame = currentFrame

        processInput(window)

        // Check for file reload
        if newFiles, err := checkFileReload(window); err == nil && len(newFiles) > 0 {
            // Cleanup old scene
            cleanupScene(&scene)
            
            // Load new scene
            scene = loadAllMeshes(program, newFiles)
            window.SetTitle(fmt.Sprintf("NavMesh Viewer - %s", strings.Join(newFiles, ", ")))
        }

        renderScene(window, program, scene)

        window.SwapBuffers()
        glfw.PollEvents()

        time.Sleep(time.Second/60 - time.Duration(deltaTime)*time.Second)
    }
}

func selectOBJFile(startDir string) (string, error) {
    var opts []zenity.Option
    opts = append(opts, zenity.Title("Select OBJ File"))
    opts = append(opts, zenity.FileFilter{
        Name:     "OBJ files",
        Patterns: []string{"*.obj"},
    })
    
    if startDir != "" {
        opts = append(opts, zenity.Filename(filepath.Join(startDir, "*.obj")))
    }
    
    filename, err := zenity.SelectFile(opts...)
    if err != nil {
        if err == zenity.ErrCanceled {
            return "", fmt.Errorf("no file selected")
        }
        return "", err
    }
    
    lastDirectory = filepath.Dir(filename)
    return filename, nil
}

func selectOBJFiles(startDir string) ([]string, error) {
    var opts []zenity.Option
    opts = append(opts, zenity.Title("Select OBJ Files"))
    opts = append(opts, zenity.FileFilter{
        Name:     "OBJ files",
        Patterns: []string{"*.obj"},
    })
    
    if startDir != "" {
        opts = append(opts, zenity.Filename(filepath.Join(startDir, "*.obj")))
    }
    
    // Use SelectFileMultiple instead of SelectFile
    filenames, err := zenity.SelectFileMultiple(opts...)
    if err != nil {
        if err == zenity.ErrCanceled {
            return nil, fmt.Errorf("no files selected")
        }
        return nil, err
    }
    
    if len(filenames) > 0 {
        lastDirectory = filepath.Dir(filenames[0])
    }
    
    return filenames, nil
}

func loadAllMeshes(program uint32, filenames []string) Scene {
    var newScene Scene
    newScene.meshes = make([]MeshData, 0, len(filenames))
    
    // Initialize bounds with first vertex of first mesh
    firstMesh := loadSingleMesh(program, filenames[0])
    newScene.bounds = firstMesh.bounds
    newScene.meshes = append(newScene.meshes, firstMesh)
    
    // Load remaining meshes and update combined bounds
    for i := 1; i < len(filenames); i++ {
        mesh := loadSingleMesh(program, filenames[i])
        newScene.meshes = append(newScene.meshes, mesh)
        newScene.bounds = combineBounds(newScene.bounds, mesh.bounds)
    }
    
    // Set camera to frame all meshes
    initializeCamera(newScene.bounds)
    return newScene
}

func loadSingleMesh(program uint32, filename string) MeshData {
    vertices, indices := loadOBJFile(filename)
    if len(vertices) == 0 || len(indices) == 0 {
        log.Printf("Warning: No mesh data loaded from %s", filename)
        return MeshData{}
    }

    bounds := calculateBounds(vertices)

    var vao uint32
    gl.GenVertexArrays(1, &vao)
    gl.BindVertexArray(vao)

    var vbo uint32
    gl.GenBuffers(1, &vbo)
    gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
    gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(vertices), gl.STATIC_DRAW)

    var ebo uint32
    gl.GenBuffers(1, &ebo)
    gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, ebo)
    gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(indices)*4, gl.Ptr(indices), gl.STATIC_DRAW)

    gl.EnableVertexAttribArray(0)
    gl.VertexAttribPointer(0, 3, gl.FLOAT, false, 0, nil)

    return MeshData{
        vertices: vertices,
        indices:  indices,
        bounds:   bounds,
        vao:      vao,
        vbo:      vbo,
        ebo:      ebo,
    }
}

func combineBounds(a, b Bounds) Bounds {
    return Bounds{
        minX: float32(math.Min(float64(a.minX), float64(b.minX))),
        minY: float32(math.Min(float64(a.minY), float64(b.minY))),
        minZ: float32(math.Min(float64(a.minZ), float64(b.minZ))),
        maxX: float32(math.Max(float64(a.maxX), float64(b.maxX))),
        maxY: float32(math.Max(float64(a.maxY), float64(b.maxY))),
        maxZ: float32(math.Max(float64(a.maxZ), float64(b.maxZ))),
    }
}

func renderScene(window *glfw.Window, program uint32, scene Scene) {
    gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
    gl.UseProgram(program)

    // Calculate view and projection matrices based on combined bounds
    sizeX := scene.bounds.maxX - scene.bounds.minX
    sizeY := scene.bounds.maxY - scene.bounds.minY
    sizeZ := scene.bounds.maxZ - scene.bounds.minZ
    maxSize := float32(math.Max(float64(sizeX), math.Max(float64(sizeY), float64(sizeZ))))

    projection := mgl32.Perspective(mgl32.DegToRad(45.0), float32(width)/float32(height), 0.1, maxSize*10)
    view := mgl32.LookAtV(cameraPos, cameraPos.Add(cameraFront), cameraUp)
    model := mgl32.Ident4()

    projectionUniform := gl.GetUniformLocation(program, gl.Str("projection\x00"))
    viewUniform := gl.GetUniformLocation(program, gl.Str("camera\x00"))
    modelUniform := gl.GetUniformLocation(program, gl.Str("model\x00"))

    gl.UniformMatrix4fv(projectionUniform, 1, false, &projection[0])
    gl.UniformMatrix4fv(viewUniform, 1, false, &view[0])
    gl.UniformMatrix4fv(modelUniform, 1, false, &model[0])

    // Render each mesh
    for _, mesh := range scene.meshes {
        gl.BindVertexArray(mesh.vao)

        // Draw solid mesh
        gl.PolygonMode(gl.FRONT_AND_BACK, gl.FILL)
        gl.Enable(gl.POLYGON_OFFSET_FILL)
        gl.PolygonOffset(1.0, 1.0)
        gl.Uniform1i(gl.GetUniformLocation(program, gl.Str("isWireframe\x00")), 0)
        gl.DrawElements(gl.TRIANGLES, int32(len(mesh.indices)), gl.UNSIGNED_INT, nil)
        gl.Disable(gl.POLYGON_OFFSET_FILL)

        // Draw wireframe overlay
        gl.PolygonMode(gl.FRONT_AND_BACK, gl.LINE)
        gl.LineWidth(1.0)
        gl.Uniform1i(gl.GetUniformLocation(program, gl.Str("isWireframe\x00")), 1)
        gl.DrawElements(gl.TRIANGLES, int32(len(mesh.indices)), gl.UNSIGNED_INT, nil)
    }

    // Reset polygon mode
    gl.PolygonMode(gl.FRONT_AND_BACK, gl.FILL)

    window.SetTitle(fmt.Sprintf("NavMesh Viewer - Speed: %.1fx", speedMultipliers[currentSpeedIndex]))
}

func cleanupScene(scene *Scene) {
    for i := range scene.meshes {
        gl.DeleteVertexArrays(1, &scene.meshes[i].vao)
        gl.DeleteBuffers(1, &scene.meshes[i].vbo)
        gl.DeleteBuffers(1, &scene.meshes[i].ebo)
    }
    scene.meshes = nil
}

func initializeWindow(filename string) (*glfw.Window, uint32) {
	glfw.WindowHint(glfw.ContextVersionMajor, 4)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.Resizable, glfw.False)
	glfw.WindowHint(glfw.Samples, 4)

	window, err := glfw.CreateWindow(width, height, fmt.Sprintf("NavMesh Viewer - %s", filename), nil, nil)
	if err != nil {
		log.Fatal(err)
	}

	window.MakeContextCurrent()
	window.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
	window.SetCursorPosCallback(mouseCallback)
	window.SetKeyCallback(keyCallback)

	if err := gl.Init(); err != nil {
		log.Fatal(err)
	}

	program := initializeShaders()
	
	gl.Enable(gl.DEPTH_TEST)
	gl.Enable(gl.CULL_FACE)
	gl.Enable(gl.MULTISAMPLE)
	gl.ClearColor(0.2, 0.2, 0.2, 1.0)

	return window, program
}

func initializeCamera(bounds Bounds) {
	// Calculate mesh center
	centerX := (bounds.minX + bounds.maxX) / 2
	centerY := (bounds.minY + bounds.maxY) / 2
	centerZ := (bounds.minZ + bounds.maxZ) / 2
	
	// Calculate mesh size
	sizeX := bounds.maxX - bounds.minX
	sizeY := bounds.maxY - bounds.minY
	sizeZ := bounds.maxZ - bounds.minZ
	maxSize := float32(math.Max(float64(sizeX), math.Max(float64(sizeY), float64(sizeZ))))
	
	// Position camera at a reasonable viewing distance
	viewDistance := maxSize * 0.8
	cameraPos = mgl32.Vec3{
		centerX,
		centerY + maxSize * 0.3,
		centerZ + viewDistance,
	}
	
	// Reset camera orientation
	yaw = -90.0
	pitch = -20.0
	
	// Update camera front vector
	direction := mgl32.Vec3{
		float32(math.Cos(float64(mgl32.DegToRad(float32(yaw)))) * math.Cos(float64(mgl32.DegToRad(float32(pitch))))),
		float32(math.Sin(float64(mgl32.DegToRad(float32(pitch))))),
		float32(math.Sin(float64(mgl32.DegToRad(float32(yaw)))) * math.Cos(float64(mgl32.DegToRad(float32(pitch))))),
	}
	cameraFront = direction.Normalize()
}

func loadAndSetupMesh(program uint32, filename string) MeshData {
	vertices, indices := loadOBJFile(filename)
	if len(vertices) == 0 || len(indices) == 0 {
		log.Fatal("No mesh data loaded")
	}

	bounds := calculateBounds(vertices)
	initializeCamera(bounds)

	var vao uint32
	gl.GenVertexArrays(1, &vao)
	gl.BindVertexArray(vao)

	var vbo uint32
	gl.GenBuffers(1, &vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(vertices), gl.STATIC_DRAW)

	var ebo uint32
	gl.GenBuffers(1, &ebo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, ebo)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(indices)*4, gl.Ptr(indices), gl.STATIC_DRAW)

	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(0, 3, gl.FLOAT, false, 0, nil)

	return MeshData{
		vertices: vertices,
		indices:  indices,
		bounds:   bounds,
		vao:      vao,
		vbo:      vbo,
		ebo:      ebo,
	}
}

func render(window *glfw.Window, program uint32, meshData MeshData) {
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	gl.UseProgram(program)

	maxSize := float32(math.Max(
		float64(meshData.bounds.maxX-meshData.bounds.minX),
		math.Max(
			float64(meshData.bounds.maxY-meshData.bounds.minY),
			float64(meshData.bounds.maxZ-meshData.bounds.minZ),
		),
	))

	projection := mgl32.Perspective(mgl32.DegToRad(45.0), float32(width)/float32(height), 0.1, maxSize*10)
	view := mgl32.LookAtV(cameraPos, cameraPos.Add(cameraFront), cameraUp)
	model := mgl32.Ident4()

	projectionUniform := gl.GetUniformLocation(program, gl.Str("projection\x00"))
	viewUniform := gl.GetUniformLocation(program, gl.Str("camera\x00"))
	modelUniform := gl.GetUniformLocation(program, gl.Str("model\x00"))

	gl.UniformMatrix4fv(projectionUniform, 1, false, &projection[0])
	gl.UniformMatrix4fv(viewUniform, 1, false, &view[0])
	gl.UniformMatrix4fv(modelUniform, 1, false, &model[0])

	gl.BindVertexArray(meshData.vao)

	// Draw solid mesh
	gl.PolygonMode(gl.FRONT_AND_BACK, gl.FILL)
	gl.Enable(gl.POLYGON_OFFSET_FILL)
	gl.PolygonOffset(1.0, 1.0)
	gl.Uniform1i(gl.GetUniformLocation(program, gl.Str("isWireframe\x00")), 0)
	gl.DrawElements(gl.TRIANGLES, int32(len(meshData.indices)), gl.UNSIGNED_INT, nil)
	gl.Disable(gl.POLYGON_OFFSET_FILL)

	// Draw wireframe overlay
	gl.PolygonMode(gl.FRONT_AND_BACK, gl.LINE)
	gl.LineWidth(1.0)
	gl.Uniform1i(gl.GetUniformLocation(program, gl.Str("isWireframe\x00")), 1)
	gl.DrawElements(gl.TRIANGLES, int32(len(meshData.indices)), gl.UNSIGNED_INT, nil)

	// Reset polygon mode
	gl.PolygonMode(gl.FRONT_AND_BACK, gl.FILL)

	window.SetTitle(fmt.Sprintf("NavMesh Viewer - Speed: %.1fx", speedMultipliers[currentSpeedIndex]))
}

func checkFileReload(window *glfw.Window) ([]string, error) {
    if window.GetKey(glfw.KeyF1) == glfw.Press {
        return selectOBJFiles(lastDirectory)
    }
    return nil, nil
}

func processInput(window *glfw.Window) {
	// Handle speed multiplier cycling
	currentShiftState := window.GetKey(glfw.KeyLeftShift)
	if currentShiftState == glfw.Press && lastShiftState == glfw.Release {
		currentSpeedIndex = (currentSpeedIndex + 1) % len(speedMultipliers)
	}
	lastShiftState = currentShiftState

	// Calculate current speed
	speed := float32(baseSpeed * speedMultipliers[currentSpeedIndex] * deltaTime)

	// Calculate right vector from camera front
	right := cameraFront.Cross(cameraUp).Normalize()

	if window.GetKey(glfw.KeyW) == glfw.Press {
		cameraPos = cameraPos.Add(cameraFront.Mul(speed))
	}
	if window.GetKey(glfw.KeyS) == glfw.Press {
		cameraPos = cameraPos.Sub(cameraFront.Mul(speed))
	}
	if window.GetKey(glfw.KeyA) == glfw.Press {
		cameraPos = cameraPos.Sub(right.Mul(speed))
	}
	if window.GetKey(glfw.KeyD) == glfw.Press {
		cameraPos = cameraPos.Add(right.Mul(speed))
	}
}

func mouseCallback(_ *glfw.Window, xpos float64, ypos float64) {
	if firstMouse {
		lastX = xpos
		lastY = ypos
		firstMouse = false
		return
	}

	xoffset := xpos - lastX
	yoffset := lastY - ypos
	lastX = xpos
	lastY = ypos

	xoffset *= mouseSensitivity
	yoffset *= mouseSensitivity

	yaw += xoffset
	pitch += yoffset

	if pitch > 89.0 {
		pitch = 89.0
	}
	if pitch < -89.0 {
		pitch = -89.0
	}

	direction := mgl32.Vec3{
		float32(math.Cos(float64(mgl32.DegToRad(float32(yaw)))) * math.Cos(float64(mgl32.DegToRad(float32(pitch))))),
		float32(math.Sin(float64(mgl32.DegToRad(float32(pitch))))),
		float32(math.Sin(float64(mgl32.DegToRad(float32(yaw)))) * math.Cos(float64(mgl32.DegToRad(float32(pitch))))),
	}
	cameraFront = direction.Normalize()
}

func keyCallback(window *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
	if key == glfw.KeyEscape && action == glfw.Press {
		window.SetShouldClose(true)
	}
}

func calculateBounds(vertices []float32) Bounds {
	bounds := Bounds{
		minX: vertices[0], maxX: vertices[0],
		minY: vertices[1], maxY: vertices[1],
		minZ: vertices[2], maxZ: vertices[2],
	}

	for i := 0; i < len(vertices); i += 3 {
		x, y, z := vertices[i], vertices[i+1], vertices[i+2]
		bounds.minX = float32(math.Min(float64(bounds.minX), float64(x)))
		bounds.maxX = float32(math.Max(float64(bounds.maxX), float64(x)))
		bounds.minY = float32(math.Min(float64(bounds.minY), float64(y)))
		bounds.maxY = float32(math.Max(float64(bounds.maxY), float64(y)))
		bounds.minZ = float32(math.Min(float64(bounds.minZ), float64(z)))
		bounds.maxZ = float32(math.Max(float64(bounds.maxZ), float64(z)))
	}

	log.Printf("Mesh bounds: X[%.2f, %.2f] Y[%.2f, %.2f] Z[%.2f, %.2f]",
		bounds.minX, bounds.maxX, bounds.minY, bounds.maxY, bounds.minZ, bounds.maxZ)
	return bounds
}

func loadOBJFile(filename string) ([]float32, []uint32) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	var vertices []float32
	var indices []uint32
	var tempVertices [][3]float32 // Temporary storage for vertices

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "v":
			if len(fields) < 4 {
				continue
			}
			x, _ := strconv.ParseFloat(fields[1], 32)
			y, _ := strconv.ParseFloat(fields[2], 32)
			z, _ := strconv.ParseFloat(fields[3], 32)
			tempVertices = append(tempVertices, [3]float32{float32(x), float32(y), float32(z)})

		case "f":
			if len(fields) < 4 {
				continue
			}
			// Convert face indices to zero-based indexing
			for i := 1; i < len(fields); i++ {
				vertexData := strings.Split(fields[i], "/")
				idx, _ := strconv.Atoi(vertexData[0])
				indices = append(indices, uint32(idx-1)) // -1 because OBJ indices are 1-based
			}
		}
	}

	// Convert temporary vertices to final format
	for _, v := range tempVertices {
		vertices = append(vertices, v[0], v[1], v[2])
	}

	log.Printf("Loaded %d vertices and %d indices", len(vertices)/3, len(indices))
	return vertices, indices
}

func initializeShaders() uint32 {
	program := gl.CreateProgram()
	
	vertexShader := compileShader(vertexShaderSource, gl.VERTEX_SHADER)
	fragmentShader := compileShader(fragmentShaderSource, gl.FRAGMENT_SHADER)

	gl.AttachShader(program, vertexShader)
	gl.AttachShader(program, fragmentShader)
	gl.LinkProgram(program)

	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLength)
		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetProgramInfoLog(program, logLength, nil, gl.Str(log))
		panic(fmt.Errorf("failed to link program: %v", log))
	}

	gl.DeleteShader(vertexShader)
	gl.DeleteShader(fragmentShader)

	return program
}

func compileShader(source string, shaderType uint32) uint32 {
	shader := gl.CreateShader(shaderType)
	csources, free := gl.Strs(source)
	gl.ShaderSource(shader, 1, csources, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)
		logText := strings.Repeat("\x00", int(logLength+1))
		gl.GetShaderInfoLog(shader, logLength, nil, gl.Str(logText))
		log.Fatal("Failed to compile shader: ", logText)
	}
	return shader
}
