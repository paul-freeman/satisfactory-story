### Acquiring `resource.json`

The following code can be run in the console of SCIM to get the resource.json file.

```javascript
JSON.stringify(Object.entries(this.SCIM.map.collectableMarkers).filter(([key]) => key.startsWith('Persistent_Level:PersistentLevel.BP_ResourceNode')).map(([, value]) => ({'id': value.options.layerId, 'lat': value._latlng.lat, 'lng': value._latlng.lng})));
```
