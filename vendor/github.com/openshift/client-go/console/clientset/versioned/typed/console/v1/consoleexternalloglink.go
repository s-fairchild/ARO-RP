// Code generated by client-gen. DO NOT EDIT.

package v1

import (
	"context"
	"time"

	v1 "github.com/openshift/api/console/v1"
	scheme "github.com/openshift/client-go/console/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// ConsoleExternalLogLinksGetter has a method to return a ConsoleExternalLogLinkInterface.
// A group's client should implement this interface.
type ConsoleExternalLogLinksGetter interface {
	ConsoleExternalLogLinks() ConsoleExternalLogLinkInterface
}

// ConsoleExternalLogLinkInterface has methods to work with ConsoleExternalLogLink resources.
type ConsoleExternalLogLinkInterface interface {
	Create(ctx context.Context, consoleExternalLogLink *v1.ConsoleExternalLogLink, opts metav1.CreateOptions) (*v1.ConsoleExternalLogLink, error)
	Update(ctx context.Context, consoleExternalLogLink *v1.ConsoleExternalLogLink, opts metav1.UpdateOptions) (*v1.ConsoleExternalLogLink, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.ConsoleExternalLogLink, error)
	List(ctx context.Context, opts metav1.ListOptions) (*v1.ConsoleExternalLogLinkList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.ConsoleExternalLogLink, err error)
	ConsoleExternalLogLinkExpansion
}

// consoleExternalLogLinks implements ConsoleExternalLogLinkInterface
type consoleExternalLogLinks struct {
	client rest.Interface
}

// newConsoleExternalLogLinks returns a ConsoleExternalLogLinks
func newConsoleExternalLogLinks(c *ConsoleV1Client) *consoleExternalLogLinks {
	return &consoleExternalLogLinks{
		client: c.RESTClient(),
	}
}

// Get takes name of the consoleExternalLogLink, and returns the corresponding consoleExternalLogLink object, and an error if there is any.
func (c *consoleExternalLogLinks) Get(ctx context.Context, name string, options metav1.GetOptions) (result *v1.ConsoleExternalLogLink, err error) {
	result = &v1.ConsoleExternalLogLink{}
	err = c.client.Get().
		Resource("consoleexternalloglinks").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ConsoleExternalLogLinks that match those selectors.
func (c *consoleExternalLogLinks) List(ctx context.Context, opts metav1.ListOptions) (result *v1.ConsoleExternalLogLinkList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1.ConsoleExternalLogLinkList{}
	err = c.client.Get().
		Resource("consoleexternalloglinks").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested consoleExternalLogLinks.
func (c *consoleExternalLogLinks) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Resource("consoleexternalloglinks").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a consoleExternalLogLink and creates it.  Returns the server's representation of the consoleExternalLogLink, and an error, if there is any.
func (c *consoleExternalLogLinks) Create(ctx context.Context, consoleExternalLogLink *v1.ConsoleExternalLogLink, opts metav1.CreateOptions) (result *v1.ConsoleExternalLogLink, err error) {
	result = &v1.ConsoleExternalLogLink{}
	err = c.client.Post().
		Resource("consoleexternalloglinks").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(consoleExternalLogLink).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a consoleExternalLogLink and updates it. Returns the server's representation of the consoleExternalLogLink, and an error, if there is any.
func (c *consoleExternalLogLinks) Update(ctx context.Context, consoleExternalLogLink *v1.ConsoleExternalLogLink, opts metav1.UpdateOptions) (result *v1.ConsoleExternalLogLink, err error) {
	result = &v1.ConsoleExternalLogLink{}
	err = c.client.Put().
		Resource("consoleexternalloglinks").
		Name(consoleExternalLogLink.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(consoleExternalLogLink).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the consoleExternalLogLink and deletes it. Returns an error if one occurs.
func (c *consoleExternalLogLinks) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return c.client.Delete().
		Resource("consoleexternalloglinks").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *consoleExternalLogLinks) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Resource("consoleexternalloglinks").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched consoleExternalLogLink.
func (c *consoleExternalLogLinks) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.ConsoleExternalLogLink, err error) {
	result = &v1.ConsoleExternalLogLink{}
	err = c.client.Patch(pt).
		Resource("consoleexternalloglinks").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}