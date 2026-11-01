# Third-Party Licenses

## Kokoro-82M ONNX Model

- **What:** the Kokoro-82M model weights and the voices it was trained with. The ONNX files and the voice archive that the worker loads (`kokoro-v1.0.onnx`, `kokoro-v1.0.int8.onnx`, `voices-v1.0.bin`) are exports and packagings of these; their own notices are in the entries below.
- **Source:** https://huggingface.co/hexgrad/Kokoro-82M
- **License:** Apache License 2.0
- **Copyright:** Copyright 2024 hexgrad

Licensed under the Apache License, Version 2.0 (the "License");
you may not use these files except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Misaki Dictionaries

- **Files:** `data/us_gold.json`, `data/us_silver.json`
- **Source:** https://github.com/hexgrad/misaki
- **License:** Apache License 2.0
- **Copyright:** Copyright 2024 hexgrad

Licensed under the Apache License, Version 2.0 (the "License");
you may not use these files except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## CMU Flite LTS Rules

- **File:** `internal/phonemizer/lts_data.go` (generated from Flite source)
- **Source:** https://github.com/festvox/flite
- **License:** BSD-like (Flite license)
- **Copyright:** Copyright (c) 1999 Carnegie Mellon University

Permission is hereby granted, free of charge, to use and distribute
this software and its documentation without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of this work, and to
permit persons to whom this work is furnished to do so, subject to
the following conditions:

1. The code must retain the above copyright notice, this list of
   conditions and the following disclaimer.
2. Any modifications must be clearly marked as such.
3. Original authors' names are not deleted.
4. The authors' names are not used to endorse or promote products
   derived from this software without specific prior written permission.

CARNEGIE MELLON UNIVERSITY AND THE CONTRIBUTORS TO THIS WORK
DISCLAIM ALL WARRANTIES WITH REGARD TO THIS SOFTWARE, INCLUDING
ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS, IN NO EVENT
SHALL CARNEGIE MELLON UNIVERSITY NOR THE CONTRIBUTORS BE LIABLE
FOR ANY SPECIAL, INDIRECT OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN
AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION,
ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF
THIS SOFTWARE.

## ONNX export of Kokoro (taylorchu/kokoro-onnx)

- **Files:** `kokoro-v1.0.onnx` and `kokoro-v1.0.int8.onnx` (the model files `make dev-models` downloads)
- **Source:** the release notes of https://github.com/thewh1teagle/kokoro-onnx/releases/tag/model-files-v1.0 say these are "optimized" f32 and int8 versions "from taylorchu/kokoro-onnx" (https://github.com/taylorchu/kokoro-onnx/releases/tag/v0.2.0)
- **License of that export:** MIT (taylorchu/kokoro-onnx, its LICENSE file copied below). The model they export is Kokoro-82M, Apache 2.0 (see the first entry).

```
MIT License

Copyright (c) 2025 taylorchu

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## Download host (thewh1teagle/kokoro-onnx)

- **What:** the repository whose `model-files-v1.0` release `make dev-models` downloads the model files and `voices-v1.0.bin` from
- **Source:** https://github.com/thewh1teagle/kokoro-onnx
- **License:** MIT (that repository's LICENSE file, copied below). This is credited because the files are fetched from, and packaged by, that repository; it is not a claim that this licence covers the model weights or the voices.

```
MIT License

Copyright (c) 2025 github.com/thewh1teagle

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## Voice style vectors

- **File:** `voices-v1.0.bin` (a numpy archive of style vectors, one per voice)
- **Provenance:** the release notes list these as the voices of Kokoro-82M and point to https://huggingface.co/hexgrad/Kokoro-82M/blob/main/VOICES.md for their names and languages. Kokoro-82M is Apache 2.0 (first entry). We have not traced who packaged the archive beyond what the release notes say.

## onnxruntime_go

- **Module:** `github.com/yalue/onnxruntime_go` v1.26.0, compiled into the worker binary
- **Source:** https://github.com/yalue/onnxruntime_go
- **License:** MIT (the module's own LICENSE file, copied below)

```
Copyright (c) 2023 Nathan Otterness

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## ONNX Runtime

- **What:** the shared library the worker loads at run time (`libonnxruntime`); the Docker image downloads release v1.24.2 from the project's GitHub releases
- **Source:** https://github.com/microsoft/onnxruntime
- **License:** MIT

```
MIT License

Copyright (c) Microsoft Corporation

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```
