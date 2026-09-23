# Using tinycode with Red Hat OpenShift AI (RHOAI)

Deploy an LLM on RHOAI with vLLM serving runtime and connect tinycode-go to it as an OpenAI-compatible provider.

## Prerequisites

- An OpenShift cluster with RHOAI installed
- At least one GPU node (NVIDIA L4, A10G, A100, or similar)
- `oc` CLI authenticated to the cluster
- `tinycode` binary built (`make build`)

## Step 1: Verify GPU availability

```bash
# Check for GPU nodes
oc get nodes -l nvidia.com/gpu.present=true \
  -o custom-columns=NAME:.metadata.name,GPU:.metadata.labels.nvidia\\.com/gpu\\.product,VRAM:.metadata.labels.nvidia\\.com/gpu\\.memory

# Check GPU count and taints
oc describe node <gpu-node-name> | grep -A2 "nvidia.com/gpu"
```

Note the GPU memory — this determines which models fit:

| GPU | VRAM | Max model (FP16) | Max model (AWQ 4-bit) |
|-----|------|-------------------|----------------------|
| L4 | 24GB | 7-8B | 27-32B |
| A10G | 24GB | 7-8B | 27-32B |
| A100 40GB | 40GB | 13-14B | 70B |
| A100 80GB | 80GB | 30B | 70B |

## Step 2: Create a data science project

```bash
oc new-project tinycode-models
oc label namespace tinycode-models opendatahub.io/dashboard=true --overwrite
```

## Step 3: Create the vLLM serving runtime

Get the correct vLLM CUDA image from your RHOAI installation:

```bash
VLLM_IMAGE=$(oc get template vllm-cuda-runtime-template \
  -n redhat-ods-applications \
  -o jsonpath='{.objects[0].spec.containers[0].image}')
echo "vLLM image: $VLLM_IMAGE"
```

Create the serving runtime. Adjust `--max-model-len` based on your GPU memory:

```bash
cat <<EOF | oc apply -n tinycode-models -f -
apiVersion: serving.kserve.io/v1alpha1
kind: ServingRuntime
metadata:
  name: vllm-cuda
spec:
  annotations:
    prometheus.io/port: "8080"
    prometheus.io/path: /metrics
  multiModel: false
  supportedModelFormats:
    - name: vLLM
      autoSelect: true
  containers:
    - name: kserve-container
      image: ${VLLM_IMAGE}
      command:
        - python
        - -m
        - vllm.entrypoints.openai.api_server
      args:
        - --port=8080
        - --model=/mnt/models
        - --served-model-name=MODEL_NAME_HERE
        - --max-model-len=32768
        - --enable-auto-tool-choice
        - --tool-call-parser=hermes
      env:
        - name: HF_HUB_OFFLINE
          value: "1"
      ports:
        - containerPort: 8080
          protocol: TCP
      resources:
        limits:
          nvidia.com/gpu: "1"
        requests:
          cpu: "2"
          memory: 8Gi
          nvidia.com/gpu: "1"
EOF
```

Replace `MODEL_NAME_HERE` with a short name for your model (e.g., `qwen2.5-7b-instruct`).

### Key vLLM arguments

| Argument | Purpose | Recommended value |
|----------|---------|-------------------|
| `--max-model-len` | Maximum context window | 32768 for 7B on 24GB; 8192 for 32B on 24GB |
| `--enable-auto-tool-choice` | Enable function/tool calling | Always include for tinycode |
| `--tool-call-parser` | Tool call format parser | `hermes` for Qwen, `llama3` for Llama |
| `--served-model-name` | Model name in the API | Used in tinycode config |
| `--quantization` | Force quantization method | `awq` or `gptq` (auto-detected if in model config) |
| `--reasoning-parser` | Parse `<think>` tags into `reasoning_content` | `qwen3` for Qwen3, `deepseek_r1` for DeepSeek |

### Enabling thinking/reasoning models

Models like Qwen3 and DeepSeek R1 produce reasoning inside `<think>...</think>` tags. By default, vLLM sends this as part of the regular `content` field — tinycode won't display it as "Thinking" output.

Add `--reasoning-parser` to make vLLM split thinking content into the `reasoning_content` response field, which tinycode displays as `+ Thought`:

```bash
args:
  - --port=8080
  - --model=/mnt/models
  - --served-model-name=qwen3-8b
  - --max-model-len=28000
  - --enable-auto-tool-choice
  - --tool-call-parser=hermes
  - --reasoning-parser=qwen3        # <-- add this for thinking models
```

Supported parsers: `qwen3`, `deepseek_r1`, `granite`, `mistral`, `glm45`, `hunyuan_a13b`, `step3`.

Without this flag, `<think>` tags appear as raw text in the assistant's response instead of being displayed as a collapsible thinking block.

**Note:** Qwen3-8B uses ~15.3 GB VRAM (vs 14.2 GB for Qwen 2.5 7B), which limits the context window to ~28,000 tokens on a 24GB GPU instead of 32,768.

## Step 4: Deploy the model

Check if your GPU node has a taint (common on dedicated GPU nodes):

```bash
oc get node <gpu-node-name> -o jsonpath='{.spec.taints}'
```

If it has `nvidia.com/gpu: NoSchedule`, include the toleration in the InferenceService.

```bash
cat <<EOF | oc apply -n tinycode-models -f -
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: my-model
  annotations:
    serving.kserve.io/deploymentMode: RawDeployment
    serving.kserve.io/autoscalerClass: none
spec:
  predictor:
    tolerations:
      - key: nvidia.com/gpu
        operator: Exists
        effect: NoSchedule
    model:
      modelFormat:
        name: vLLM
      runtime: vllm-cuda
      storageUri: hf://Qwen/Qwen2.5-7B-Instruct
    minReplicas: 1
    maxReplicas: 1
EOF
```

### Recommended models for tinycode

| Model | HuggingFace ID | VRAM (FP16) | Tool calling | Notes |
|-------|---------------|-------------|-------------|-------|
| Qwen 2.5 7B Instruct | `Qwen/Qwen2.5-7B-Instruct` | ~14GB | Yes (hermes) | Best for 24GB GPUs |
| Qwen 3 8B | `Qwen/Qwen3-8B` | ~16GB | Yes (hermes) | Newer, with thinking |
| Qwen 2.5 Coder 7B | `Qwen/Qwen2.5-Coder-7B-Instruct` | ~14GB | Yes | Code-optimized |
| Granite 3.1 8B | `ibm-granite/granite-3.1-8b-instruct` | ~16GB | Yes | IBM's model |
| Mistral 7B v0.3 | `mistralai/Mistral-7B-Instruct-v0.3` | ~14GB | Yes | Solid general purpose |
| Qwen 2.5 32B AWQ | `Qwen/Qwen2.5-32B-Instruct-AWQ` | ~18GB | Yes | 4-bit, limited context |

## Step 5: Wait for the model to load

The init container downloads the model from HuggingFace (can take 5-15 minutes depending on model size and network speed), then vLLM loads it into GPU memory (1-2 minutes).

```bash
# Watch pod status
oc get pods -n tinycode-models -w

# Check download progress
oc logs <pod-name> -n tinycode-models -c storage-initializer -f

# Check vLLM loading progress
oc logs <pod-name> -n tinycode-models -c kserve-container -f
```

The model is ready when you see:
```
INFO:     Application startup complete.
```

## Step 6: Expose the model endpoint

Create a service and route for external access:

```bash
# Create a routable service (KServe's default service is headless)
cat <<EOF | oc apply -n tinycode-models -f -
apiVersion: v1
kind: Service
metadata:
  name: my-model-api
spec:
  selector:
    app: isvc.my-model-predictor
  ports:
    - port: 8080
      targetPort: 8080
  type: ClusterIP
EOF

# Create an edge-terminated route (HTTPS)
oc create route edge my-model --service=my-model-api --port=8080 -n tinycode-models

# Get the route URL
ROUTE=$(oc get route my-model -n tinycode-models -o jsonpath='{.spec.host}')
echo "Model endpoint: https://${ROUTE}/v1"
```

### Verify the endpoint

```bash
# List models
curl -sk "https://${ROUTE}/v1/models" | python3 -m json.tool

# Test a chat completion
curl -sk "https://${ROUTE}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "MODEL_NAME_HERE",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 50
  }' | python3 -m json.tool
```

## Step 7: Configure tinycode

Add the RHOAI endpoint to your tinycode config at `~/.config/tinycode/tinycode.jsonc`:

```jsonc
{
  "provider": {
    "rhoai": {
      "options": {
        "name": "RHOAI",
        "baseURL": "https://YOUR_ROUTE_HOST/v1"
      },
      "models": {
        "MODEL_NAME_HERE": {
          "limit": {
            "context": 32768,
            "output": 4096
          }
        }
      }
    }
  }
}
```

Replace `YOUR_ROUTE_HOST` with the route hostname from Step 6, and `MODEL_NAME_HERE` with the `--served-model-name` from Step 3.

## Step 8: Connect tinycode

```bash
# Start tinycode
./dist/tinycode

# Use /connect to select the RHOAI provider
# Or start with the model pre-selected:
./dist/tinycode -m rhoai/MODEL_NAME_HERE
```

## Managing multiple models

With a single GPU, you can only run one model at a time. Deploy multiple InferenceServices and swap by scaling:

```bash
# Switch to Qwen 2.5 7B
oc scale deployment qwen25-7b-predictor -n tinycode-models --replicas=1
oc scale deployment qwen3-8b-predictor -n tinycode-models --replicas=0

# Switch to Qwen3 8B
oc scale deployment qwen25-7b-predictor -n tinycode-models --replicas=0
oc scale deployment qwen3-8b-predictor -n tinycode-models --replicas=1
```

Note: each model needs its own service and route. The model will take 2-3 minutes to start after scaling up (loading weights into GPU memory; the HuggingFace download is cached).

## Changing context window size

Update the `--max-model-len` argument in the ServingRuntime:

```bash
# Find the current args
oc get servingruntime vllm-cuda -n tinycode-models -o jsonpath='{.spec.containers[0].args}'

# Patch to a new context size
oc patch servingruntime vllm-cuda -n tinycode-models --type='json' \
  -p='[{"op": "replace", "path": "/spec/containers/0/args/3", "value": "--max-model-len=32768"}]'

# Restart the deployment (scale down then up — rolling restart fails with 1 GPU)
oc scale deployment my-model-predictor -n tinycode-models --replicas=0
sleep 15
oc scale deployment my-model-predictor -n tinycode-models --replicas=1
```

Context size vs GPU memory trade-off:
- Larger context = less KV cache = fewer concurrent requests
- For single-user tinycode, concurrency doesn't matter — maximize context

| Model size | 24GB GPU max context | 40GB GPU max context |
|------------|---------------------|---------------------|
| 7B FP16 | 32768 | 65536 |
| 8B FP16 | 32768 | 65536 |
| 32B AWQ | 4096-8192 | 16384-32768 |

## Model caching and persistence

By default, KServe's storage initializer downloads the model from HuggingFace into an `emptyDir` volume every time a pod starts. When the pod is deleted (scaling, swapping models, restarts), the download is lost and must repeat (~2-15 minutes depending on model size).

### Option 1: Accept re-download (sandbox/dev)

For sandbox environments with good internet, the 2-minute re-download per swap is acceptable. No configuration needed — this is the default behavior.

### Option 2: Pre-download to MinIO (production)

The standard RHOAI pattern for model persistence uses MinIO or S3 object storage. Download the model once, store it in a bucket, then reference it with an `s3://` URI.

**Set up MinIO** (if not already deployed):

```bash
# RHOAI's Data Science Pipelines often deploy MinIO automatically.
# Check if MinIO is already available:
oc get pods --all-namespaces | grep minio

# If not, deploy MinIO:
cat <<EOF | oc apply -n tinycode-models -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: minio
spec:
  replicas: 1
  selector:
    matchLabels:
      app: minio
  template:
    metadata:
      labels:
        app: minio
    spec:
      containers:
        - name: minio
          image: quay.io/minio/minio:latest
          args: ["server", "/data", "--console-address", ":9001"]
          env:
            - name: MINIO_ROOT_USER
              value: minioadmin
            - name: MINIO_ROOT_PASSWORD
              value: minioadmin
          ports:
            - containerPort: 9000
            - containerPort: 9001
          volumeMounts:
            - name: data
              mountPath: /data
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: minio-data
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: minio-data
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 50Gi
---
apiVersion: v1
kind: Service
metadata:
  name: minio
spec:
  selector:
    app: minio
  ports:
    - name: api
      port: 9000
    - name: console
      port: 9001
EOF
```

**Download model to MinIO**:

```bash
# Port-forward to MinIO
oc port-forward svc/minio 9000:9000 -n tinycode-models &

# Install mc (MinIO client) if needed: brew install minio/stable/mc
mc alias set local http://localhost:9000 minioadmin minioadmin

# Create a bucket and download the model
mc mb local/models
# Use huggingface-cli to download, then upload to MinIO:
pip install huggingface_hub
huggingface-cli download Qwen/Qwen2.5-7B-Instruct --local-dir /tmp/qwen25-7b
mc cp --recursive /tmp/qwen25-7b/ local/models/qwen25-7b/
```

**Create a storage secret**:

```bash
cat <<EOF | oc apply -n tinycode-models -f -
apiVersion: v1
kind: Secret
metadata:
  name: minio-creds
  annotations:
    serving.kserve.io/s3-endpoint: minio.tinycode-models.svc:9000
    serving.kserve.io/s3-usehttps: "0"
stringData:
  AWS_ACCESS_KEY_ID: minioadmin
  AWS_SECRET_ACCESS_KEY: minioadmin
EOF
```

**Reference the MinIO model in InferenceService**:

```yaml
spec:
  predictor:
    serviceAccountName: default
    model:
      modelFormat:
        name: vLLM
      runtime: vllm-cuda
      storageUri: s3://models/qwen25-7b
      storage:
        key: minio-creds
```

With this setup, the model loads from local MinIO storage in seconds instead of downloading from HuggingFace each time.

### Option 3: OCI container image (air-gapped)

Bake model weights into a container image. Best for air-gapped environments where external downloads aren't possible. Slow to build (~30 minutes for a 14GB model) but instant to deploy.

```bash
# Build a model image
cat <<EOF > Containerfile.model
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
RUN microdnf install -y python3.12-pip && pip3 install huggingface_hub
RUN huggingface-cli download Qwen/Qwen2.5-7B-Instruct --local-dir /models
EOF
podman build -t quay.io/yourorg/qwen25-7b:latest -f Containerfile.model .
podman push quay.io/yourorg/qwen25-7b:latest
```

Then reference with `storageUri: oci://quay.io/yourorg/qwen25-7b:latest`.

### Why PVCs don't work directly

KServe's webhook manages the `/mnt/models` volume mount through its storage initializer init container. Adding a PVC volume mount at `/mnt/models` on the InferenceService spec conflicts with KServe's own volume management, resulting in: `admission webhook denied the request: unable to determine storage type`. The MinIO/S3 approach works because it goes through KServe's storage initializer protocol.

## Troubleshooting

### Pod stuck in Pending
```bash
oc describe pod <pod-name> -n tinycode-models | grep -A5 Events
```
Common causes:
- **GPU taint**: Add `tolerations` to the InferenceService spec
- **Insufficient GPU**: Another pod is using the GPU — scale it down first
- **Insufficient memory**: Increase node memory or use a smaller model

### HTTP 503 from route
The pod is not ready yet, or the service selector doesn't match. Check:
```bash
oc get pods -n tinycode-models  # Is the pod 1/1 Running?
oc get endpoints my-model-api -n tinycode-models  # Does the endpoint have an IP?
```

### Context length exceeded
```
ValueError: This model's maximum context length is 8192 tokens.
However, your request has 12000 input tokens.
```
Increase `--max-model-len` in the ServingRuntime (see "Changing context window size" above).

### Model download fails
Check the init container logs:
```bash
oc logs <pod-name> -c storage-initializer -n tinycode-models
```
Common causes:
- HuggingFace rate limiting — wait and retry
- Model requires authentication — set `HF_TOKEN` env var in the ServingRuntime
- Network issues — check cluster egress

### tinycode shows "max retries exceeded"
The model endpoint is unreachable. Check:
```bash
curl -sk https://YOUR_ROUTE_HOST/v1/models
```
If this returns HTML instead of JSON, the route is broken — recreate the service and route (Step 6).

### Permission warnings in init container
```
Permission denied: '/mnt/tmp_...'
```
These are cosmetic — the HuggingFace cache can't set permissions but the download continues. Safe to ignore.

## Using tinycode doctor

After configuring the provider, verify connectivity:

```bash
./dist/tinycode doctor
```

Look for the RHOAI provider in the output:
```
✓ Provider: rhoai — 1 model (qwen2.5-7b-instruct)
```
