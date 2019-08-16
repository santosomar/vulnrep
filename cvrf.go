// Copyright 2019, TIBCO Software Inc. All Rights Reserved

package vulnrep

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	// NamespaceV11 is the namespace for the 1.1 version of the cvrfdoc XML format.
	namespaceV11 = "http://www.icasi.org/CVRF/schema/cvrf/1.1"

	// NamespaceV12 is the namespace for the 1.2 version of the cvrfdoc XML format.
	namespaceV12 = "http://docs.oasis-open.org/csaf/ns/csaf-cvrf/v1.2/cvrf"
)

// reportV11 is a clone of reportV12, except with different annotations for the fields.
type reportV11 struct {
	XMLName           xml.Name              `xml:"http://www.icasi.org/CVRF/schema/cvrf/1.1 cvrfdoc"`
	Title             string                `xml:"DocumentTitle"`
	Type              string                `xml:"DocumentType"`
	Publisher         publisherExp          `xml:"DocumentPublisher"`
	Tracking          trackingExp           `xml:"DocumentTracking"`
	DocumentNotes     []noteExp             `xml:"DocumentNotes>Note"`
	Distribution      string                `xml:"DocumentDistribution,omitempty"`
	AggregateSeverity *aggregateSeverityExp `xml:"AggregateSeverity,omitempty"`
	References        []referenceExp        `xml:"DocumentReferences>Reference,omitempty"`
	ProductTree       productTreeExp        `xml:"http://www.icasi.org/CVRF/schema/prod/1.1 ProductTree"`
	Vulnerabilities   []vulnerabilityXML    `xml:"http://www.icasi.org/CVRF/schema/vuln/1.1 Vulnerability"`
}

// reportV12 is the root structure for parsing a CVRF XML format document.
type reportV12 struct {
	XMLName           xml.Name              `xml:"http://docs.oasis-open.org/csaf/ns/csaf-cvrf/v1.2/cvrf cvrfdoc"`
	Title             string                `xml:"DocumentTitle"`
	Type              string                `xml:"DocumentType"`
	Publisher         publisherExp          `xml:"DocumentPublisher"`
	Tracking          trackingExp           `xml:"DocumentTracking"`
	DocumentNotes     []noteExp             `xml:"DocumentNotes>Note"`
	Distribution      string                `xml:"DocumentDistribution,omitempty"`
	AggregateSeverity *aggregateSeverityExp `xml:"AggregateSeverity,omitempty"`
	References        []referenceExp        `xml:"DocumentReferences>Reference,omitempty"`
	Acknowledgments   []acknowledgmentExp   `xml:"Acknowledgments>Acknowledgment,omitempty"`
	ProductTree       productTreeExp        `xml:"http://docs.oasis-open.org/csaf/ns/csaf-cvrf/v1.2/prod ProductTree"`
	Vulnerabilities   []vulnerabilityXML    `xml:"http://docs.oasis-open.org/csaf/ns/csaf-cvrf/v1.2/vuln Vulnerability"`
}

// xmlToModel converts the vulnerability report from CVRF (XML format) to the all-
// encompassing internal model.
func (r *reportV12) asReport() (Report, loadCtx) {

	var ctx loadCtx
	var res Report

	// parse all the metadata for the document
	res.Meta.Title = r.Title
	res.Meta.Type = r.Type
	res.Meta.Publisher = Publisher(r.Publisher)
	res.Meta.Tracking = r.Tracking.asTracking()

	for _, nx := range r.DocumentNotes {
		res.Meta.Notes = append(res.Meta.Notes, Note(nx))
	}

	res.Meta.Distribution = r.Distribution
	res.Meta.AggregateSeverity = r.AggregateSeverity.asAggregateSeverity()

	for _, rx := range r.References {
		res.Meta.References = append(res.Meta.References, Reference(rx))
	}

	for _, ack := range r.Acknowledgments {
		res.Meta.Acknowledgments = append(res.Meta.Acknowledgments, Acknowledgment(ack))
	}

	res.ProductTree = r.ProductTree.asProductTree(&ctx)

	for _, vx := range r.Vulnerabilities {
		res.Vulnerabilities = append(res.Vulnerabilities, vx.asVulnerability(&ctx))
	}
	return res, ctx
}

func toReportXML(r Report) reportV12 {

	refs := make([]referenceExp, 0, len(r.Meta.References))
	for _, ref := range r.Meta.References {
		refs = append(refs, referenceExp(ref))
	}
	acks := make([]acknowledgmentExp, 0, len(r.Meta.Acknowledgments))
	for _, ack := range r.Meta.Acknowledgments {
		acks = append(acks, acknowledgmentExp(ack))
	}
	vulns := make([]vulnerabilityXML, 0, len(r.Vulnerabilities))
	for _, vuln := range r.Vulnerabilities {
		vulns = append(vulns, toVulnerabilityXML(vuln))
	}

	return reportV12{
		Title:             r.Meta.Title,
		Type:              r.Meta.Type,
		Publisher:         publisherExp(r.Meta.Publisher),
		Tracking:          toTrackingExp(r.Meta.Tracking),
		DocumentNotes:     toNotesXML(r.Meta.Notes),
		Distribution:      r.Meta.Distribution,
		AggregateSeverity: toAggregateSeverityExp(r.Meta.AggregateSeverity),
		References:        refs,
		Acknowledgments:   acks,
		ProductTree:       toProductTreeXML(r.ProductTree),
		Vulnerabilities:   vulns}
}

// PublisherXML captures publisher information from a CVRF document
type publisherExp struct {
	Type             PublisherType `xml:"Type,attr" json:"type"`
	VendorID         string        `xml:"VendorID,attr,omitempty" json:"vendor_id,omitempty"`
	ContactDetails   string        `xml:"ContactDetails" json:"contact_details"`
	IssuingAuthority string        `xml:"IssuingAuthority" json:"issuing_authority"`
}

// trackingXML captures the tracking data for a CVRF document
type trackingExp struct {
	ID                 string        `xml:"Identification>ID" json:"id"`
	Aliases            []string      `xml:"Identification>Alias,omitempty" json:"aliases,omitempty"`
	Status             DocStatus     `xml:"Status" json:"status"`
	Version            RevisionStr   `xml:"Version" json:"version"`
	Revisions          []revisionExp `xml:"RevisionHistory>Revision" json:"revisions"`
	InitialReleaseDate time.Time     `xml:"InitialReleaseDate" json:"initial_release_date"`
	CurrentReleaseDate time.Time     `xml:"CurrentReleaseDate" json:"current_release_date"`
	Generator          *generatorExp `xml:"Generator,omitempty" json:"generator,omitempty"`
}

func toTrackingExp(t Tracking) trackingExp {

	return trackingExp{
		ID:                 t.ID,
		Aliases:            t.Aliases,
		Status:             t.Status,
		Version:            t.Version,
		Revisions:          toRevisionExps(t.Revisions),
		InitialReleaseDate: t.InitialReleaseDate,
		CurrentReleaseDate: t.CurrentReleaseDate,
		Generator:          toGeneratorExp(t.Generator),
	}
}

func (t trackingExp) asTracking() Tracking {
	convRevs := make([]Revision, 0, len(t.Revisions))
	for _, rev := range t.Revisions {
		convRevs = append(convRevs, Revision(rev))
	}
	return Tracking{
		ID:                 t.ID,
		Aliases:            t.Aliases,
		Status:             t.Status,
		Version:            t.Version,
		Revisions:          convRevs,
		InitialReleaseDate: t.InitialReleaseDate,
		CurrentReleaseDate: t.CurrentReleaseDate,
		Generator:          t.Generator.asGenerator(),
	}
}

type generatorExp struct {
	Engine string    `xml:"Engine,omitempty" json:"engine,omitempty"`
	Date   time.Time `xml:"Date,omitempty" json:"date"`
}

func toGeneratorExp(g *Generator) *generatorExp {
	if g == nil {
		return nil
	}
	result := generatorExp(*g)
	return &result
}

func (g *generatorExp) asGenerator() *Generator {
	if g == nil {
		return nil
	}
	result := Generator(*g)
	return &result
}

// aggregateSeverityXML captures the aggregate severity information for CVRF document
type aggregateSeverityExp struct {
	Namespace string `xml:"Namespace,attr,omitempty" json:"namespace,omitempty"`
	Text      string `xml:",chardata" json:"text"`
}

func (asx *aggregateSeverityExp) asAggregateSeverity() *AggregateSeverity {
	if asx == nil {
		return nil
	}
	result := AggregateSeverity(*asx)
	return &result
}

func toAggregateSeverityExp(as *AggregateSeverity) *aggregateSeverityExp {
	if as == nil {
		return nil
	}
	result := aggregateSeverityExp(*as)
	return &result
}

// revisionXML captures the xml representation of document revisions.
type revisionExp struct {
	Number      RevisionStr `xml:"Number" json:"number"`
	Date        time.Time   `xml:"Date" json:"date"`
	Description string      `xml:"Description" json:"description"`
}

func toRevisionExps(revs []Revision) []revisionExp {
	result := make([]revisionExp, 0, len(revs))
	for _, rev := range revs {
		result = append(result, revisionExp(rev))
	}
	return result
}

// noteXML captures the document level notes of a CVRF document
type noteExp struct {
	Title    string   `xml:"Title,attr,omitempty" json:"title,omitempty"`
	Audience string   `xml:"Audience,attr,omitempty" json:"audience,omitempty"`
	Type     NoteType `xml:"Type,attr" json:"type"`
	Ordinal  int      `xml:"Ordinal,attr" json:"ordinal"`
	Text     string   `xml:",chardata" json:"text"`
}

func toNotesXML(notes []Note) []noteExp {
	result := make([]noteExp, 0, len(notes))
	for _, note := range notes {
		result = append(result, noteExp(note))
	}
	return result
}

// referenceXML captures document level references in CVRF
type referenceExp struct {
	Type        ReferenceType `xml:"Type,attr,omitempty" json:"type,omitempty"`
	URL         string        `xml:"URL" json:"url"`
	Description string        `xml:"Description" json:"description"`
}

func toReferenceExps(refs []Reference) []referenceExp {
	result := make([]referenceExp, 0, len(refs))
	for _, ref := range refs {
		result = append(result, referenceExp(ref))
	}
	return result
}

// acknowledgmentXML captures acknowledgments for the XML format.
type acknowledgmentExp struct {
	Names         []string `xml:"Name" json:"names"`
	Organizations []string `xml:"Organization" json:"organizations"`
	Description   string   `xml:"Description,omitempty" json:"description"`
	URLs          []string `xml:"URL" json:"urls"`
}

func toAcknowledgmentExps(acks []Acknowledgment) []acknowledgmentExp {
	result := make([]acknowledgmentExp, 0, len(acks))
	for _, ack := range acks {
		result = append(result, acknowledgmentExp(ack))
	}
	return result
}

// ProductTreeXML captures the XML representation of the CVRF product tree
type productTreeExp struct {
	Branches         []branchExp       `xml:"Branch,omitempty" json:"branches,omitempty"`
	FullProductNames []fullProductExp  `xml:"FullProductName,omitempty" json:"full_product_names"`
	Relationships    []relationshipExp `xml:"Relationship,omitempty" json:"relationships"`
	ProductGroups    []groupExp        `xml:"ProductGroups>Group,omitempty" json:"product_groups"`
}

func toProductTreeXML(pt ProductTree) productTreeExp {

	branches := make([]branchExp, 0, len(pt.Branches)+len(pt.Leaves))
	for _, br := range pt.Branches {
		branches = append(branches, toBranchExp(br))
	}
	for _, leaf := range pt.Leaves {
		branches = append(branches, toSingleBranch(leaf))
	}

	products := make([]fullProductExp, 0, len(pt.Products))
	for _, prd := range pt.Products {
		products = append(products, toFullProductExp(prd))
	}

	relationships := make([]relationshipExp, 0, len(pt.Relationships))
	for _, rel := range pt.Relationships {
		relationships = append(relationships, toRelationshipExp(rel))
	}

	groups := make([]groupExp, 0, len(pt.Groups))
	for _, grp := range pt.Groups {
		groups = append(groups, toGroupExp(*grp))
	}

	return productTreeExp{
		Branches:         branches,
		FullProductNames: products,
		Relationships:    relationships,
		ProductGroups:    groups}
}

// enumProducts gathers all the products listed in FullProductNames, Branches,
// and Relationships, and creates a single list of them
func (ptx *productTreeExp) enumProducts() []*Product {

	var results []*Product
	for _, fp := range ptx.FullProductNames {
		results = append(results, fp.asProduct())
	}

	for _, br := range ptx.Branches {
		results = br.allProducts(results)
	}

	for _, rel := range ptx.Relationships {
		for _, fpn := range rel.FullProductNames {
			results = append(results, fpn.asProduct())
		}
	}
	return results
}

// asProductTree converts a ProductTreeXML structure into a ProductTree,
// mapping data elements as needed.
func (ptx productTreeExp) asProductTree(ctx *loadCtx) ProductTree {

	var result ProductTree
	prods := ptx.enumProducts()
	ctx.prodMap = make(map[ProductID]*Product)
	for _, prd := range prods {
		ctx.prodMap[prd.ID] = prd
	}
	// create groups, using the product lookup map.
	ctx.groupMap = make(map[GroupID]*Group)
	for _, xmlGrp := range ptx.ProductGroups {
		newGrp := xmlGrp.asGroup(ctx.prodMap)
		ctx.groupMap[newGrp.ID] = newGrp
		result.Groups = append(result.Groups, newGrp)
	}

	// convert the top level products to a list of products
	for _, fpn := range ptx.FullProductNames {
		result.Products = append(result.Products, ctx.prodMap[fpn.ProductID])
	}

	// convert the "branches" into branches and leaves.
	result.Branches, result.Leaves = asBranchesAndLeaves(ctx.prodMap, ptx.Branches)

	// convert relationships
	for _, rel := range ptx.Relationships {
		result.Relationships = append(result.Relationships,
			rel.asRelationship(ctx))
	}
	return result
}

func asBranchesAndLeaves(prods map[ProductID]*Product, expBranches []branchExp) ([]Branch, []ProductLeaf) {
	var resultLeaves []ProductLeaf
	var resultBranches []Branch
	for _, branch := range expBranches {
		if branch.Product != nil {

			// find the product from the first round of gathering, so we're using the
			// same instance of the product, rather than creating a new one.
			foundProd := prods[branch.Product.ProductID]
			toAdd := ProductLeaf{
				Name:    branch.Name,
				Type:    branch.Type,
				Product: foundProd}
			resultLeaves = append(resultLeaves, toAdd)
		} else {
			childBranches, childLeaves := asBranchesAndLeaves(prods, branch.Branches)
			toAdd := Branch{
				Name:     branch.Name,
				Type:     branch.Type,
				Branches: childBranches,
				Leaves:   childLeaves}
			resultBranches = append(resultBranches, toAdd)
		}
	}
	return resultBranches, resultLeaves
}

// branchXML captures the XML representation of branches in the product tree
type branchExp struct {
	Type     BranchType      `xml:"Type,attr" json:"type"`
	Name     string          `xml:"Name,attr" json:"name"`
	Branches []branchExp     `xml:"Branch,omitempty" json:"branches,omitempty"`
	Product  *fullProductExp `xml:"FullProductName,omitempty" json:"product,omitempty"`
}

func toBranchExp(br Branch) branchExp {
	branches := make([]branchExp, 0, len(br.Branches)+len(br.Leaves))
	for _, child := range br.Branches {
		branches = append(branches, toBranchExp(child))
	}
	for _, leaf := range br.Leaves {
		branches = append(branches, toSingleBranch(leaf))
	}
	return branchExp{
		Name:     br.Name,
		Type:     br.Type,
		Branches: branches,
		Product:  nil}
}

func toSingleBranch(leaf ProductLeaf) branchExp {

	prod := toFullProductExp(leaf.Product)
	return branchExp{
		Name:     leaf.Name,
		Type:     leaf.Type,
		Branches: nil,
		Product:  &prod}
}

func (bx *branchExp) allProducts(list []*Product) []*Product {
	if bx.Product != nil {
		list = append(list, bx.Product.asProduct())
	} else {
		for _, br := range bx.Branches {
			list = br.allProducts(list)
		}
	}
	return list
}

// fullProductXML captures the XML representation of the full product description
type fullProductExp struct {
	ProductID ProductID `xml:"ProductID,attr" json:"product_id"`
	CPE       string    `xml:"CPE,attr,omitempty" json:"cpe,omitempty"`
	Name      string    `xml:",chardata" json:"name"`
}

func toFullProductExp(prd *Product) fullProductExp {
	return fullProductExp{
		ProductID: prd.ID,
		CPE:       prd.CPE,
		Name:      prd.Name}
}

func (fpx fullProductExp) asProduct() *Product {
	return &Product{
		ID:   fpx.ProductID,
		CPE:  fpx.CPE,
		Name: fpx.Name}
}

// relationshipXML captures the XML representation of the relationship component of
// the product tree.
type relationshipExp struct {
	ProductReference          ProductID        `xml:"ProductReference,attr" json:"product_reference"`
	RelationshipType          RelationshipType `xml:"RelationType,attr" json:"relationship_type"`
	RelatesToProductReference ProductID        `xml:"RelatesToProductReference,attr" json:"relates_to_product_reference"`
	FullProductNames          []fullProductExp `xml:"FullProductName" json:"full_product_names"`
}

func toRelationshipExp(rel Relationship) relationshipExp {

	products := make([]fullProductExp, 0, len(rel.Products))
	for _, prd := range rel.Products {
		products = append(products, toFullProductExp(prd))
	}

	return relationshipExp{
		RelationshipType:          rel.Type,
		ProductReference:          rel.Reference.ID,
		RelatesToProductReference: rel.RelatesToReference.ID,
		FullProductNames:          products}
}

func (rx relationshipExp) asRelationship(ctx *loadCtx) Relationship {

	var prods []*Product
	for _, fpn := range rx.FullProductNames {
		prods = append(prods, ctx.prodMap[fpn.ProductID])
	}

	return Relationship{
		Type:               rx.RelationshipType,
		Products:           prods,
		Reference:          ctx.prodMap[rx.ProductReference],
		RelatesToReference: ctx.prodMap[rx.RelatesToProductReference]}
}

// groupXML captures the XML representation of a product grouping.
type groupExp struct {
	GroupID     GroupID     `xml:"GroupID,attr" json:"group_id"`
	Description string      `xml:"Description,omitempty" json:"description"`
	ProductIDs  []ProductID `xml:"ProductID" json:"product_ids"` // at least two required
}

func toGroupExp(grp Group) groupExp {
	return groupExp{
		GroupID:     grp.ID,
		Description: grp.Description,
		ProductIDs:  toProductIDs(grp.Products)}
}

func (gx groupExp) asGroup(lookup map[ProductID]*Product) *Group {

	var prodList []*Product
	for _, id := range gx.ProductIDs {
		prodList = append(prodList, lookup[id])
	}

	return &Group{
		Description: gx.Description,
		ID:          gx.GroupID,
		Products:    prodList}
}

// vulnerabilityXML captures the XML information of a vulnerability
type vulnerabilityXML struct {
	Ordinal         int                 `xml:"Ordinal,attr"` // positive integer
	Title           string              `xml:"Title,omitempty"`
	ID              *vulnIDExp          `xml:"ID,omitempty"`
	Notes           []noteExp           `xml:"Notes>Note"`
	DiscoveryDate   time.Time           `xml:"DiscoveryDate,omitempty"`
	ReleaseDate     time.Time           `xml:"ReleaseDate,omitempty"`
	Involvements    []involvementExp    `xml:"Involvements>Involvement,omitempty"`
	CVE             string              `xml:"CVE,omitempty"`
	CWE             *cweExp             `xml:"CWE,omitempty"`
	Statuses        []statusXML         `xml:"ProductStatuses>Status,omitempty"`
	Threats         []threatExp         `xml:"Threats>Threat,omitempty"`
	CVSSScoreSets   *cvssScoreSetsXML   `xml:"CVSSScoreSets,omitempty"`
	Remediations    []remediationExp    `xml:"Remediations>Remediation"`
	References      []referenceExp      `xml:"References>Reference,omitempty"`
	Acknowledgments []acknowledgmentExp `xml:"Acknowledgments>Acknowledgment,omitempty"`
}

func toVulnerabilityXML(v Vulnerability) vulnerabilityXML {

	return vulnerabilityXML{
		Ordinal:         v.Ordinal,
		Title:           v.Title,
		ID:              toVulnIDExp(v.ID),
		Notes:           toNotesXML(v.Notes),
		DiscoveryDate:   v.DiscoveryDate,
		ReleaseDate:     v.ReleaseDate,
		Involvements:    toInvolvmentExps(v.Involvements),
		CVE:             v.CVE,
		CWE:             toCWEExp(v.CWE),
		Statuses:        toStatusXML(v.Statuses),
		Threats:         toThreatExps(v.Threats),
		CVSSScoreSets:   toCVSSScoreSetsXML(v.CVSS),
		Remediations:    toRemediationExps(v.Remediations),
		References:      toReferenceExps(v.References),
		Acknowledgments: toAcknowledgmentExps(v.Acknowledgments)}
}

func (vx vulnerabilityXML) asVulnerability(ctx *loadCtx) Vulnerability {

	notes := make([]Note, 0, len(vx.Notes))
	for _, nx := range vx.Notes {
		notes = append(notes, Note(nx))
	}

	involvements := make([]Involvement, 0, len(vx.Involvements))
	for _, ix := range vx.Involvements {
		involvements = append(involvements, Involvement(ix))
	}

	threats := make([]Threat, 0, len(vx.Threats))
	for _, th := range vx.Threats {
		threats = append(threats, th.asThreat(ctx))
	}

	remediations := make([]Remediation, 0, len(vx.Remediations))
	for _, rm := range vx.Remediations {
		remediations = append(remediations, rm.asRemediation(ctx))
	}

	refs := make([]Reference, 0, len(vx.References))
	for _, ref := range vx.References {
		refs = append(refs, Reference(ref))
	}

	acks := make([]Acknowledgment, 0, len(vx.Acknowledgments))
	for _, ack := range vx.Acknowledgments {
		acks = append(acks, Acknowledgment(ack))
	}

	return Vulnerability{
		Ordinal:         vx.Ordinal,
		Title:           vx.Title,
		ID:              vx.ID.asVulnID(),
		Notes:           notes,
		DiscoveryDate:   vx.DiscoveryDate,
		ReleaseDate:     vx.ReleaseDate,
		Involvements:    involvements,
		CVE:             vx.CVE,
		CWE:             vx.CWE.asCWEExp(),
		Statuses:        asStatus(ctx, vx.Statuses),
		Threats:         threats,
		CVSS:            vx.CVSSScoreSets.asScoreSet(ctx),
		Remediations:    remediations,
		References:      refs,
		Acknowledgments: acks}
}

// idXML captures the XML identifier for a vulnerabilIty
type vulnIDExp struct {
	SystemName string `xml:"SystemName,attr" json:"system_name"`
	ID         string `xml:",chardata" json:"text"`
}

func toVulnIDExp(vid *VulnID) *vulnIDExp {
	if vid == nil {
		return nil
	}
	result := vulnIDExp(*vid)
	return &result
}

func (vid *vulnIDExp) asVulnID() *VulnID {
	if vid == nil {
		return nil
	}
	result := VulnID(*vid)
	return &result
}

// cweXML corresponds to the XML serialization of the CWE data.
type cweExp struct {
	ID          string `xml:"ID,attr" json:"id"`
	Description string `xml:",chardata" json:"description"`
}

func toCWEExp(cwe *CWE) *cweExp {
	if cwe == nil {
		return nil
	}
	result := &cweExp{
		ID:          cwe.ID,
		Description: cwe.Description}
	return result
}

func (cwe *cweExp) asCWEExp() *CWE {
	if cwe == nil {
		return nil
	}
	result := CWE(*cwe)
	return &result
}

// statusExp captures the list of all products with a given status.
type statusXML struct {
	Type       AffectedStatusType `xml:"Type,attr"`
	ProductIDs []ProductID        `xml:"ProductID"`
}

func oneStatus(result []statusXML, ast AffectedStatusType, prods []*Product) []statusXML {
	if len(prods) == 0 {
		return result
	}
	return append(result, statusXML{Type: ast, ProductIDs: toProductIDs(prods)})
}

func toStatusXML(status Status) []statusXML {

	result := oneStatus(nil, AffectedStatusFirstAffected, status.FirstAffected)
	result = oneStatus(result, AffectedStatusFirstFixed, status.FirstFixed)
	result = oneStatus(result, AffectedStatusFixed, status.Fixed)
	result = oneStatus(result, AffectedStatusKnownAffected, status.KnownAffected)
	result = oneStatus(result, AffectedStatusKnownNotAffected, status.KnownNotAffected)
	result = oneStatus(result, AffectedStatusLastAffected, status.LastAffected)
	result = oneStatus(result, AffectedStatusRecommended, status.Recommended)

	return result
}

func asStatus(ctx *loadCtx, statuses []statusXML) Status {

	var result Status
	for _, st := range statuses {
		var list *[]*Product
		var msg string
		switch st.Type {
		case AffectedStatusFirstAffected:
			list, msg = &result.FirstAffected, "first affected"
		case AffectedStatusFirstFixed:
			list, msg = &result.FirstFixed, "first fixed"
		case AffectedStatusFixed:
			list, msg = &result.Fixed, "fixed"
		case AffectedStatusKnownAffected:
			list, msg = &result.KnownAffected, "known affected"
		case AffectedStatusKnownNotAffected:
			list, msg = &result.KnownNotAffected, "known not affected"
		case AffectedStatusLastAffected:
			list, msg = &result.LastAffected, "last affected"
		case AffectedStatusRecommended:
			list, msg = &result.Recommended, "recommended"
		}
		*list = append(*list, ctx.asProducts(st.ProductIDs, msg)...)
	}
	return result
}

// InvolvementXML captures the list of involvements for a particular vulnerability
type involvementExp struct {
	Party       PublisherType         `xml:"Party,attr" json:"party"`
	Status      InvolvementStatusType `xml:"Status,attr" json:"status"`
	Description string                `xml:"Description,omitempty" json:"description,omitempty"`
}

func toInvolvmentExps(invs []Involvement) []involvementExp {
	result := make([]involvementExp, 0, len(invs))
	for _, inv := range invs {
		result = append(result, involvementExp(inv))
	}
	return result
}

// ThreatXML captures the XML representation of the threat types
type threatExp struct {
	Type        ThreatType  `xml:"Type,attr" json:"type"`
	Description string      `xml:"Description" json:"description"`
	Date        time.Time   `xml:"Date,attr,omitempty" json:"date"`
	ProductIDs  []ProductID `xml:"ProductID,omitempty" json:"products"`
	GroupIDs    []GroupID   `xml:"GroupID,omitempty" json:"groups"`
}

func toThreatExp(th Threat) threatExp {
	groups := make([]GroupID, 0, len(th.Groups))
	for _, grp := range th.Groups {
		groups = append(groups, grp.ID)
	}

	return threatExp{
		Type:        th.Type,
		Description: th.Description,
		Date:        th.Date,
		ProductIDs:  toProductIDs(th.Products),
		GroupIDs:    groups}
}

func toThreatExps(threats []Threat) []threatExp {
	result := make([]threatExp, 0, len(threats))
	for _, th := range threats {
		result = append(result, toThreatExp(th))
	}
	return result
}

func (tx threatExp) asThreat(ctx *loadCtx) Threat {
	return Threat{
		Type:        tx.Type,
		Description: tx.Description,
		Date:        tx.Date,
		Products:    ctx.asProducts(tx.ProductIDs, "threat"),
		Groups:      ctx.asGroups(tx.GroupIDs, "threats"),
	}
}

// CVSSScoreSetsXML captures the XML representation of possible CVSS scores,
// either v2 or v3.
type cvssScoreSetsXML struct {
	ScoreSetV2 []scoreSetV2XML `xml:"ScoreSetV2,omitempty"`
	ScoreSetV3 []scoreSetV3Exp `xml:"ScoreSetV3,omitempty"`
}

func toCVSSScoreSetsXML(ss *CVSSScoreSets) *cvssScoreSetsXML {
	if ss == nil {
		return nil
	}
	v2scores := make([]scoreSetV2XML, 0, len(ss.V2))
	for _, sv2 := range ss.V2 {
		v2scores = append(v2scores, scoreSetV2XML(toScoreSetV3Exp(sv2)))
	}
	result := cvssScoreSetsXML{
		ScoreSetV2: v2scores,
		ScoreSetV3: toScoreSetV3Exps(ss.V3)}
	return &result
}

func (css *cvssScoreSetsXML) asScoreSet(ctx *loadCtx) *CVSSScoreSets {
	if css == nil {
		return nil
	}

	v2s := make([]ScoreSet, 0, len(css.ScoreSetV2))
	for _, v2 := range css.ScoreSetV2 {
		v2s = append(v2s, v2.asScoreSet(ctx))
	}

	v3s := make([]ScoreSet, 0, len(css.ScoreSetV3))
	for _, v3 := range css.ScoreSetV3 {
		v3s = append(v3s, v3.asScoreSet(ctx))
	}

	return &CVSSScoreSets{
		V2: v2s,
		V3: v3s}
}

// scoreSetV2XML captures the XML representation of the CVSS v3 scoring.
type scoreSetV2XML struct {
	BaseScore          string      `xml:"BaseScoreV2"`
	TemporalScore      string      `xml:"TemporalScoreV2,omitempty"`
	EnvironmentalScore string      `xml:"EnvironmentalScoreV2,omitempty"`
	Vector             string      `xml:"VectorV2,omitempty"`
	ProductIDs         []ProductID `xml:"ProductID,omitempty"`
}

func (ssx scoreSetV2XML) asScoreSet(ctx *loadCtx) ScoreSet {
	return scoreSetV3Exp(ssx).asScoreSet(ctx)
}

// scoreSetV3XML captures the XML representation of the CVSS v3 scoring.
type scoreSetV3Exp struct {
	BaseScore          string      `xml:"BaseScoreV3" json:"base_score"`
	TemporalScore      string      `xml:"TemporalScoreV3,omitempty" json:"temporal_score"`
	EnvironmentalScore string      `xml:"EnvironmentalScoreV3,omitempty" json:"environmental_score"`
	Vector             string      `xml:"VectorV3" json:"vector"`
	ProductIDs         []ProductID `xml:"ProductID,omitempty" json:"product_ids"`
}

func toScoreSetV3Exp(ss ScoreSet) scoreSetV3Exp {
	return scoreSetV3Exp{
		BaseScore:          ss.BaseScore,
		TemporalScore:      ss.TemporalScore,
		EnvironmentalScore: ss.EnvironmentalScore,
		Vector:             ss.Vector,
		ProductIDs:         toProductIDs(ss.Products)}
}

func toScoreSetV3Exps(ss []ScoreSet) []scoreSetV3Exp {
	result := make([]scoreSetV3Exp, 0, len(ss))
	for _, sv3 := range ss {
		result = append(result, toScoreSetV3Exp(sv3))
	}
	return result
}

func (ssx scoreSetV3Exp) asScoreSet(ctx *loadCtx) ScoreSet {
	return ScoreSet{
		BaseScore:          ssx.BaseScore,
		TemporalScore:      ssx.TemporalScore,
		EnvironmentalScore: ssx.EnvironmentalScore,
		Vector:             ssx.Vector,
		Products:           ctx.asProducts(ssx.ProductIDs, "cvss score")}
}

// remediationExp captures the XML representation for remediations of a vulnerability
type remediationExp struct {
	Type        RemedyType  `xml:"Type,attr" json:"type"`
	Date        time.Time   `xml:"Date,attr" json:"date"`
	Description string      `xml:"Description" json:"description"`
	Entitlement []string    `xml:"Entitlement,omitempty" json:"entitlement"`
	URL         string      `xml:"URL,omitempty" json:"url"`
	Products    []ProductID `xml:"ProductID,omitempty" json:"products"`
	Groups      []GroupID   `xml:"GroupID,omitempty" json:"groups"`
}

func (rem remediationExp) asRemediation(ctx *loadCtx) Remediation {

	return Remediation{
		Type:        rem.Type,
		Date:        rem.Date,
		Description: rem.Description,
		Entitlement: rem.Entitlement,
		URL:         rem.URL,
		Products:    ctx.asProducts(rem.Products, "remediation"),
		Groups:      ctx.asGroups(rem.Groups, "remediation"),
	}
}

func toRemediationExp(rem Remediation) remediationExp {
	return remediationExp{
		Type:        rem.Type,
		Date:        rem.Date,
		Description: rem.Description,
		Entitlement: rem.Entitlement,
		URL:         rem.URL,
		Products:    toProductIDs(rem.Products),
		Groups:      toGroupIDs(rem.Groups),
	}
}

func toRemediationExps(rems []Remediation) []remediationExp {
	result := make([]remediationExp, 0, len(rems))
	for _, rem := range rems {
		result = append(result, toRemediationExp(rem))
	}
	return result
}

// LoadErr captures the errors that might occur during load
type LoadErr struct {
	Issues []string
}

func (le *LoadErr) Error() string {
	return strings.Join(le.Issues, "\n")
}

type loadCtx struct {
	issues   []string
	prodMap  map[ProductID]*Product
	groupMap map[GroupID]*Group
}

func (lc *loadCtx) err(msg string) {
	lc.issues = append(lc.issues, msg)
}

func (lc *loadCtx) asProduct(id ProductID, loc string) *Product {
	prd := lc.prodMap[id]
	if prd == nil {
		lc.err(fmt.Sprintf("unable to find product id %v for %v", id, loc))
	}
	return prd
}

func (lc *loadCtx) asProducts(list []ProductID, loc string) []*Product {
	var result []*Product
	for _, id := range list {
		result = append(result, lc.asProduct(id, loc))
	}
	return result
}

func (lc *loadCtx) asGroups(list []GroupID, loc string) []*Group {
	var result []*Group
	for _, id := range list {
		grp := lc.groupMap[id]
		if grp == nil {
			lc.err(fmt.Sprintf("unable to find group id %v for %v", id, loc))
		}
		result = append(result, grp)
	}
	return result
}

// ParseXMLFile parses an expected file...
func ParseXML(r io.Reader) (Report, error) {

	var emptyReport Report

	data, err := ioutil.ReadAll(r)
	if err != nil {
		return emptyReport, errors.Wrap(err, "problem opening for parse")
	}

	se, err := firstElement(data)
	if err != nil {
		return emptyReport, err
	}
	if se.Name.Local != "cvrfdoc" {
		return emptyReport, fmt.Errorf("expected root element cvrfdoc, got %v", se.Name.Local)
	}

	var doc reportV12
	if se.Name.Space == namespaceV11 {
		var doc11 reportV11
		err = xml.Unmarshal(data, &doc11)
		// copy over to the 1.2 version of the data.
		doc = reportV12(doc)
	} else if se.Name.Space == namespaceV12 {
		err = xml.Unmarshal(data, &doc)
	}
	if err != nil {
		return emptyReport, errors.Wrap(err, "problem unmarshalling XML")
	}

	res, ctx := doc.asReport()
	if len(ctx.issues) > 0 {
		return res, &LoadErr{Issues: ctx.issues}
	}
	return res, nil
}

func firstElement(rawFile []byte) (xml.StartElement, error) {

	buf := bytes.NewBuffer(rawFile)
	d := xml.NewDecoder(buf)
	for {
		t, err := d.Token()
		if err == io.EOF {
			return xml.StartElement{}, errors.New("did not get an element before EOF")
		}
		if err != nil {
			return xml.StartElement{}, err
		}
		if se, ok := t.(xml.StartElement); ok {
			return se, nil
		}
	}
}

func toProductIDs(prods []*Product) []ProductID {
	ids := make([]ProductID, 0, len(prods))
	for _, prd := range prods {
		ids = append(ids, prd.ID)
	}
	return ids
}

func toGroupIDs(grps []*Group) []GroupID {
	ids := make([]GroupID, 0, len(grps))
	for _, grp := range grps {
		ids = append(ids, grp.ID)
	}
	return ids
}
