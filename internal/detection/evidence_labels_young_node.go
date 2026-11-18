package detection

// EvidenceYoungNode is the node a failure's pods run on and how many
// minutes after it joined they began failing ("n7 3"), when that was
// less than the node boot window. The incident manager adds it.
const EvidenceYoungNode = "young node"
