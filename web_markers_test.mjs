import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const html = readFileSync(new URL('./web/index.html', import.meta.url), 'utf8');
const section = (start, end) => {
    const from = html.indexOf(start);
    const to = html.indexOf(end, from);
    assert.ok(from >= 0 && to > from, `could not locate ${start}`);
    return html.slice(from, to);
};

test('cached markers remain visible when a recent peer report shares their location', () => {
    const rendered = [];
    const context = {
        L: {
            divIcon: options => options,
            marker: (point, options) => ({
                point, options,
                bindPopup(popup) { this.popup = popup; return this; },
            }),
        },
        markers: {
            clearLayers() { rendered.length = 0; },
            addLayer(marker) { rendered.push(marker); },
        },
        formatTime: () => 'now',
    };
    vm.createContext(context);
    vm.runInContext(
        section('function createNodeIcon(current', '// Connection arcs') +
        section('function hasGeo(node)', 'function updateConnections(nodes)'),
        context,
    );

    const nodes = [
        { peer_id: 'recent-report', ip: '192.0.2.1', lat: 1, lng: 1, recently_seen: true, in_last_crawl: false },
        { peer_id: 'cached-overlap', ip: '192.0.2.2', lat: 1, lng: 1, recently_seen: false, in_last_crawl: false },
        { peer_id: 'cached-only', ip: '192.0.2.3', lat: 2, lng: 2, recently_seen: false, in_last_crawl: false },
    ];
    context.nodes = nodes;
    vm.runInContext('updateMap(nodes)', context);

    assert.equal(rendered.length, 9); // Three world copies of each node.
    const originalWorld = rendered.filter(marker => marker.point[1] === 1 || marker.point[1] === 2);
    assert.equal(originalWorld.length, 3);
    const byIP = Object.fromEntries(originalWorld.map(marker => [
        marker.popup.match(/192\.0\.2\.\d/)[0], marker.options,
    ]));
    assert.equal(byIP['192.0.2.1'].pane, 'currentNodes');
    assert.equal(byIP['192.0.2.2'].pane, 'cachedNodes');
    assert.equal(byIP['192.0.2.3'].pane, 'cachedNodes');
    assert.equal(byIP['192.0.2.1'].icon.iconSize[0], 10);
    assert.equal(byIP['192.0.2.2'].icon.iconSize[0], 24); // Outer ring remains visible.
    assert.equal(byIP['192.0.2.3'].icon.iconSize[0], 10);
});
