D2_SOURCES := $(wildcard docs/diagrams/*.d2)
D2_SVGS := $(D2_SOURCES:.d2=.svg)

.PHONY: diagrams clean-diagrams

diagrams: $(D2_SVGS)

docs/diagrams/%.svg: docs/diagrams/%.d2
	d2 --pad=20 $< $@

clean-diagrams:
	rm -f $(D2_SVGS)
