# gotq V1 conformance fixtures

`queries.json` is a transport/parser fixture set for downstream adapters. Each
case supplies decoded `url.Values`, whether compatibility aliases are enabled,
and the expected stable error code and parameter for rejected input.

These fixtures deliberately stop before model binding: public fields and
operators are endpoint policy, while V1 syntax is shared by every endpoint.
Consumers may copy the file, vendor it, or execute it directly from the module.
Additive cases may appear in patch releases; existing case semantics do not
change within syntax version V1.
