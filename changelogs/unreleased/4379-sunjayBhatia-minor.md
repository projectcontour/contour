## Re-increase maximum allowed regex program size

Regex patterns Contour configures in Envoy (for path matching etc.) currently have a limited "program size" (approximate cost) of 100.
This was inadvertently set back to the Envoy default, from the intended 1048576 (2^20) when moving away from using deprecated API fields.
Note: regex program size is a feature of the regex library Envoy uses, [Google RE2](https://github.com/google/re2).

This limit has now been reset to the intended value and an additional program size warning threshold of 1000 has been configured.

Operators concerned with performance implications of allowing large regex programs can monitor Envoy memory usage and regex statistics.
Envoy offers two statistics for monitoring regex program size, `re2.program_size` and `re2.exceeded_warn_level`.
See [this documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/type/matcher/v3/regex.proto.html?highlight=warn_level#type-matcher-v3-regexmatcher-googlere2) for more detail.
Future versions of Contour may allow configuration of regex program size thresholds via RTDS (Runtime Discovery Service).
