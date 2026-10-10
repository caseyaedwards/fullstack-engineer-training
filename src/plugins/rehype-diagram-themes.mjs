// Runs after rehype-mermaid (strategy: img-svg, dark: true). Replaces each
// <picture><source dark><img light></picture> with two <img> elements classed
// `diagram-light` / `diagram-dark`; src/styles/diagrams.css shows one based on
// Starlight's data-theme. Reuse the same classes for other build-time diagrams.
function walk(node) {
  if (!node.children) return;
  node.children = node.children.flatMap((child) => {
    if (child.type === 'element' && child.tagName === 'picture') {
      const source = child.children.find((c) => c.type === 'element' && c.tagName === 'source');
      const img = child.children.find((c) => c.type === 'element' && c.tagName === 'img');
      if (source && img) {
        const { media, srcset, ...rest } = source.properties ?? {};
        const dark = {
          type: 'element',
          tagName: 'img',
          properties: { ...rest, alt: img.properties?.alt ?? '', src: srcset, className: ['diagram-dark'] },
          children: [],
        };
        const light = {
          ...img,
          properties: { ...img.properties, className: ['diagram-light'] },
        };
        return [light, dark];
      }
    }
    walk(child);
    return [child];
  });
}

export default function rehypeDiagramThemes() {
  return (tree) => walk(tree);
}
